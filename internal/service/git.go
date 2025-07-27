package service

import (
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

func (b *Builder) cloneRepository() error {
	dir, err := os.ReadDir(b.cfg.WorkDir)
	if errors.Is(err, os.ErrNotExist) {
		slog.Info("work directory does not exist, creating", "workDir", b.cfg.WorkDir)
		if err := os.MkdirAll(b.cfg.WorkDir, 0755); err != nil {
			return err
		}
		return b.cloneRepository()
	}
	if err != nil {
		return err
	}
	if len(dir) > 0 {
		slog.Info("work directory is not empty, skipping clone", "workDir", b.cfg.WorkDir)
		return nil
	}

	cmd := exec.Command("git", "clone", "--branch", b.cfg.RepoBranch, b.cfg.RepoURL, b.cfg.WorkDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (b *Builder) hasChanges() (bool, error) {
	cmd := exec.Command("git", "-C", b.cfg.WorkDir, "fetch", "origin", b.cfg.RepoBranch)
	if err := cmd.Run(); err != nil {
		return false, err
	}

	cmd = exec.Command("git", "-C", b.cfg.WorkDir, "rev-parse", "HEAD")
	localOutput, err := cmd.Output()
	if err != nil {
		return false, err
	}
	localCommit := strings.TrimSpace(string(localOutput))
	b.lastCommit = localCommit

	cmd = exec.Command("git", "-C", b.cfg.WorkDir, "rev-parse", "origin/"+b.cfg.RepoBranch)
	remoteOutput, err := cmd.Output()
	if err != nil {
		return false, err
	}
	remoteCommit := strings.TrimSpace(string(remoteOutput))

	return localCommit != remoteCommit, nil
}

func (b *Builder) updateRepository() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	var err error
	for attempts := 0; attempts < 3; attempts++ {
		if err = b.tryUpdateRepository(); err == nil {
			break
		}
		slog.Warn("Git operation failed, retrying", "attempt", attempts+1, "error", err)
		time.Sleep(time.Duration(attempts+1) * time.Second)
	}
	return err
}

func (b *Builder) tryUpdateRepository() error {

	hasChanges, err := b.hasChanges()
	if err != nil {
		return err
	}

	if !hasChanges {
		slog.Info("No changes detected, skipping update")
		return nil
	}

	cmd := exec.Command("git", "-C", b.cfg.WorkDir, "fetch", "origin", b.cfg.RepoBranch)
	cmd.Dir = b.cfg.WorkDir
	if err := cmd.Run(); err != nil {
		return err
	}

	cmd = exec.Command("git", "-C", b.cfg.WorkDir, "reset", "--hard", "origin/"+b.cfg.RepoBranch)
	cmd.Dir = b.cfg.WorkDir
	if err := cmd.Run(); err != nil {
		return err
	}

	cmd = exec.Command("git", "-C", b.cfg.WorkDir, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return err
	}

	oldCommit := b.lastCommit
	b.lastCommit = strings.TrimSpace(string(output))
	slog.Info("Repository updated", "oldCommit", oldCommit, "newCommit", b.lastCommit)

	return nil
}
