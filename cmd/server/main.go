package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nnip7777/ssh-vpn/internal/channel"
	"github.com/nnip7777/ssh-vpn/internal/config"
	"github.com/nnip7777/ssh-vpn/internal/logutil"
	"github.com/nnip7777/ssh-vpn/internal/ssh"
	"github.com/nnip7777/ssh-vpn/internal/tun"
	"github.com/nnip7777/ssh-vpn/internal/version"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	configPath := flag.String("config", "server.yaml", "path to config file")
	showVersion := flag.Bool("version", false, "show version and exit")
	generateKey := flag.Bool("generate-key", false, "generate host key")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.String())
		return
	}

	logger, logWriter := logutil.NewLogger("log", "ssh-vpn-server", zapcore.InfoLevel)
	defer logger.Sync()
	defer logWriter.Close()

	logger.Info("starting", zap.String("version", version.Short()))

	if *generateKey {
		if err := ssh.GenerateHostKey("host_key"); err != nil {
			logger.Fatal("failed to generate host key", zap.Error(err))
		}
		logger.Info("host key generated: host_key")
		return
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	logger.Info("starting SSH VPN server",
		zap.String("listen", fmt.Sprintf("%s:%d", cfg.Server.ListenAddr, cfg.Server.ListenPort)))

	sshServerConfig, err := ssh.NewSSHServerConfig(&ssh.ServerConfig{
		HostKeyPath:        "host_key",
		AuthorizedKeysPath: "authorized_keys",
		MaxAuthTries:       3,
	}, logger)
	if err != nil {
		logger.Fatal("failed to create SSH config", zap.Error(err))
	}

	tunIface, err := tun.New(tun.Config{
		Name:    cfg.Server.TUNName,
		Addr:    cfg.Server.TUNAddr,
		Netmask: cfg.Server.TUNNetmask,
		MTU:     cfg.Server.MTU,
	}, logger)
	if err != nil {
		logger.Fatal("failed to create TUN interface", zap.Error(err))
	}
	defer tunIface.Close()

	channelMgr := channel.NewManager(
		cfg.Channels.MinRead,
		cfg.Channels.MaxRead,
		cfg.Channels.MinWrite,
		cfg.Channels.MaxWrite,
		cfg.Channels.ReadRatio,
		cfg.Channels.WriteRatio,
		cfg.Channels.HealthCheck,
		cfg.Channels.Timeout,
		logger,
	)

	addr := fmt.Sprintf("%s:%d", cfg.Server.ListenAddr, cfg.Server.ListenPort)
	server, err := ssh.NewServer(addr, sshServerConfig, channelMgr, tunIface, logger)
	if err != nil {
		logger.Fatal("failed to create SSH server", zap.Error(err))
	}
	defer server.Close()

	for _, extraPort := range cfg.Server.ExtraPorts {
		extraAddr := fmt.Sprintf("%s:%d", cfg.Server.ListenAddr, extraPort)
		if err := server.AddListener(extraAddr); err != nil {
			logger.Fatal("failed to add extra listener", zap.Error(err))
		}
	}

	server.Start()
	go server.BroadcastToClients()

	go func() {
		if err := server.Accept(); err != nil {
			logger.Error("server accept error", zap.Error(err))
		}
	}()

	logger.Info("server started",
		zap.String("addr", addr),
		zap.Ints("extra_ports", cfg.Server.ExtraPorts),
		zap.String("tun", cfg.Server.TUNName),
		zap.Int("max_clients", cfg.Server.MaxClients))

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	<-sigCh
	logger.Info("shutting down...")

	if err := server.WaitForShutdown(30); err != nil {
		logger.Warn("shutdown timeout", zap.Error(err))
	}

	logger.Info("server stopped")
}

func statsReporter(server *ssh.Server, mgr *channel.Manager, logger *zap.Logger, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var prevTotalIn, prevTotalOut uint64
	prevTime := time.Now()

	for range ticker.C {
		clients := server.GetClientCount()
		stats := mgr.GetStats()
		readCount, writeCount := mgr.ChannelCount()

		var totalIn, totalOut, totalErrors uint64
		for _, s := range stats {
			totalIn += s.BytesRecv
			totalOut += s.BytesSent
			totalErrors += s.Errors
		}

		now := time.Now()
		elapsed := now.Sub(prevTime).Seconds()
		if elapsed < 1 {
			elapsed = 1
		}
		deltaIn := totalIn - prevTotalIn
		deltaOut := totalOut - prevTotalOut
		if totalIn < prevTotalIn {
			deltaIn = 0
		}
		if totalOut < prevTotalOut {
			deltaOut = 0
		}

		prevTotalIn = totalIn
		prevTotalOut = totalOut
		prevTime = now

		logger.Info("stats",
			zap.Int("clients", clients),
			zap.Int("read_ch", readCount),
			zap.Int("write_ch", writeCount),
			zap.String("throughput_in", fmtBytes(float64(deltaIn)/elapsed)),
			zap.String("throughput_out", fmtBytes(float64(deltaOut)/elapsed)),
			zap.Uint64("total_in", totalIn),
			zap.Uint64("total_out", totalOut),
			zap.Uint64("errors", totalErrors),
		)
	}
}

func fmtBytes(b float64) string {
	const (
		KB = 1024
		MB = KB * 1024
	)
	switch {
	case b >= MB:
		return fmt.Sprintf("%.1fMB/s", b/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1fKB/s", b/float64(KB))
	default:
		return fmt.Sprintf("%.0fB/s", b)
	}
}
