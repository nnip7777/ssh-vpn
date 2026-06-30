package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nnip7777/ssh-vpn/internal/client"
	"github.com/nnip7777/ssh-vpn/internal/config"
	"github.com/nnip7777/ssh-vpn/internal/logutil"
	"github.com/nnip7777/ssh-vpn/internal/version"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	configPath := flag.String("config", "client.yaml", "path to config file")
	showVersion := flag.Bool("version", false, "show version and exit")
	logLevel := flag.String("log", "info", "log level: debug, info, warn, error")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.String())
		return
	}

	var lvl zapcore.Level
	switch strings.ToLower(*logLevel) {
	case "debug":
		lvl = zapcore.DebugLevel
	case "warn", "warning":
		lvl = zapcore.WarnLevel
	case "error":
		lvl = zapcore.ErrorLevel
	default:
		lvl = zapcore.InfoLevel
	}

	logger, logWriter := logutil.NewLogger("log", "ssh-vpn-client", lvl)
	defer logger.Sync()
	defer logWriter.Close()

	logger.Info("starting", zap.String("version", version.Short()))

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	c, err := client.New(cfg, logger)
	if err != nil {
		logger.Fatal("failed to create client", zap.Error(err))
	}

	if err := c.Connect(); err != nil {
		logger.Fatal("failed to connect to server", zap.Error(err))
	}

	if err := c.Start(); err != nil {
		logger.Fatal("failed to start client", zap.Error(err))
	}

	logger.Info("client started",
		zap.String("server", fmt.Sprintf("%s:%d", cfg.Client.ServerAddr, cfg.Client.ServerPort)),
		zap.String("tun", cfg.Client.TUNName))

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	<-sigCh
	logger.Info("shutting down...")

	c.Stop()

	logger.Info("client stopped")
}

func statsReporter(c *client.Client, logger *zap.Logger, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var prevTotalIn, prevTotalOut uint64
	prevTime := time.Now()

	for range ticker.C {
		if !c.IsConnected() {
			continue
		}

		stats := c.GetStats()
		connected, _ := stats["connected"].(bool)
		if !connected {
			continue
		}

		chStats, _ := stats["channel_stats"].(map[uint16]struct {
			BytesSent   uint64
			BytesRecv   uint64
			PacketsSent uint64
			PacketsRecv uint64
			Errors      uint64
		})

		readCount, _ := stats["read_channels"].(int)
		writeCount, _ := stats["write_channels"].(int)

		var totalIn, totalOut, totalErrors uint64
		for _, s := range chStats {
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
