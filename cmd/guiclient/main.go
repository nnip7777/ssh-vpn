package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/nnip7777/ssh-vpn/internal/config"
	"github.com/nnip7777/ssh-vpn/internal/gui"
	"github.com/nnip7777/ssh-vpn/internal/version"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	configPath := flag.String("config", "client.yaml", "path to config file")
	showVersion := flag.Bool("version", false, "show version and exit")
	flag.Parse()

	if *showVersion {
		println(version.String())
		return
	}

	logger := initLogger()
	defer logger.Sync()

	logger.Info("starting", zap.String("version", version.Short()))

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	a := gui.New(cfg, logger)

	if cfg.Client.AutoConnect {
		go a.StartClient()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		logger.Info("shutting down...")
		a.StopClient()
		os.Exit(0)
	}()

	a.Run()
}

func initLogger() *zap.Logger {
	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	logger, err := cfg.Build()
	if err != nil {
		panic(err)
	}

	return logger
}
