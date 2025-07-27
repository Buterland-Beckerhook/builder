package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

func (b *Builder) startWebhookServer() {
	http.HandleFunc("/health", b.handleHealthCheck)
	http.HandleFunc("/webhook", b.handleWebhook)

	slog.Info("Starting webhook server", "address", b.cfg.ServerAddress)

	b.server = &http.Server{
		Addr: b.cfg.ServerAddress,
	}

	if err := b.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("Failed to start webhook server", "error", err)
		return
	}
}

func (b *Builder) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":      "healthy",
		"last_commit": b.lastCommit,
	})
}

func (b *Builder) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body := make([]byte, r.ContentLength)
	_, _ = r.Body.Read(body)

	if !b.verifySignature(r, body) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if !b.shouldTriggerBuild(body) {
		slog.Info("Webhook received but branch doesn't match, skipping build")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	slog.Info("Received webhook request, triggering build")
	go b.updateAndBuild()

	w.WriteHeader(http.StatusAccepted)
}

func (b *Builder) verifySignature(r *http.Request, body []byte) bool {
	if b.cfg.WebhookSecret == "" {
		return true // Skip verification if no secret is set
	}

	signature := r.Header.Get("X-Hub-Signature-256")
	if signature == "" {
		return false
	}

	mac := hmac.New(sha256.New, []byte(b.cfg.WebhookSecret))
	mac.Write(body)
	expectedSignature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

func (b *Builder) shouldTriggerBuild(body []byte) bool {
	var payload struct {
		Ref string `json:"ref"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		slog.Warn("Failed to parse webhook payload", "error", err)
		return true
	}

	expectedRef := "refs/heads/" + b.cfg.RepoBranch
	return payload.Ref == expectedRef
}
