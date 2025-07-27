package main

import (
	"bb-builder/internal/config"
	"bb-builder/internal/service"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	cfg, err := config.New()
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	builder := service.NewBuilder(cfg)

	if err := builder.Start(); err != nil {
		slog.Error("Failed to start builder", "error", err)
		os.Exit(1)
	} else {
		slog.Info("Builder started successfully")
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	slog.Info("Received signal, shutting down", "signal", sig)

	builder.Stop()
}
