package service

import (
	"bb-builder/internal/config"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Builder struct {
	cfg *config.Config

	lastCommit string
	server     *http.Server
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.RWMutex
	isBuilding bool
}

func NewBuilder(cfg *config.Config) *Builder {
	ctx, cancel := context.WithCancel(context.Background())
	return &Builder{
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
	}
}

func (b *Builder) Start() error {
	err := b.initialBuild()
	if err != nil {
		return err
	}

	go b.startWebhookServer()

	if b.cfg.PollInterval > 0 {
		go b.startPolling()
	}

	return nil
}

func (b *Builder) Stop() {
	b.cancel()

	if b.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		slog.Info("Stopping webhook server")
		if err := b.server.Shutdown(ctx); err != nil {
			slog.Error("Error stopping webhook server", "error", err)
		} else {
			slog.Info("Webhook server stopped successfully")
		}
	}
}

func (b *Builder) initialBuild() error {
	slog.Info("Starting initial build")

	err := b.cloneRepository()
	if err != nil {
		return err
	}

	go b.updateAndBuild()
	return nil
}

func (b *Builder) updateAndBuild() {
	select {
	case <-b.ctx.Done():
		slog.Info("Build cancelled due to shutdown")
		return
	default:
	}

	// Atomic build flag
	b.mu.Lock()
	if b.isBuilding {
		b.mu.Unlock()
		slog.Info("Build already in progress, skipping")
		return
	}
	b.isBuilding = true
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		b.isBuilding = false
		b.mu.Unlock()
	}()

	err := b.updateRepository()
	if err != nil {
		slog.Error("Failed to update repository", "error", err)
		return
	}

	err = b.buildSite()
	if err != nil {
		slog.Error("Failed to build site", "error", err)
		return
	}
}

func (b *Builder) buildSite() error {
	start := time.Now()
	slog.Info("Building site", "commit", b.lastCommit)

	buildID := fmt.Sprintf("build-%d", time.Now().Unix())
	buildDir := filepath.Join(b.cfg.OutputDir, ".builds", buildID)

	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return err
	}

	args := append([]string{"--destination", buildDir}, b.cfg.HugoArgs...)

	cmd := exec.Command("hugo", args...)
	cmd.Dir = b.cfg.WorkDir

	output, err := cmd.CombinedOutput()
	duration := time.Since(start)

	if err != nil {
		_ = os.RemoveAll(buildDir)
		slog.Error("Failed to build site",
			"duration", duration,
			"output", string(output),
			"error", err,
		)
		return err
	}

	if err := b.atomicSwap(buildDir); err != nil {
		_ = os.RemoveAll(buildDir)
		return err
	}

	go b.cleanupOldBuilds()

	slog.Info("Site built successfully", "duration", duration, "buildID", buildID,
		"args", args)
	return nil
}

func (b *Builder) atomicSwap(newBuildDir string) error {
	currentLink := filepath.Join(b.cfg.OutputDir, "current")
	tempLink := filepath.Join(b.cfg.OutputDir, "current.tmp")

	buildID := filepath.Base(newBuildDir)
	relativePath := filepath.Join(".builds", buildID)

	if err := os.Symlink(relativePath, tempLink); err != nil {
		return err
	}

	if err := os.Rename(tempLink, currentLink); err != nil {
		_ = os.Remove(tempLink)
		return err
	}

	return nil
}

func (b *Builder) cleanupOldBuilds() {
	buildsDir := filepath.Join(b.cfg.OutputDir, ".builds")
	entries, err := os.ReadDir(buildsDir)
	if err != nil {
		return
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() > entries[j].Name()
	})

	for i := 2; i < len(entries); i++ {
		oldBuild := filepath.Join(buildsDir, entries[i].Name())
		err := os.RemoveAll(oldBuild)
		if err != nil {
			slog.Error("Failed to remove old build", "path", oldBuild, "error", err)
			continue
		}
		slog.Info("Cleaned up old build", "path", oldBuild)
	}
}

func (b *Builder) startPolling() {
	ticker := time.NewTicker(time.Duration(b.cfg.PollInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			slog.Info("Polling stopped due to shutdown")
			return
		case <-ticker.C:
			slog.Info("Polling for changes")
			go b.updateAndBuild()
		}
	}
}
