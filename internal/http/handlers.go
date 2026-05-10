// Package http exposes the bot's control endpoints (start / pause / stop /
// status). All mutating endpoints are guarded by RequireAPIKey.
package http

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/wu-piyaphon/outbound-api/internal/bot"
)

// BotHandlers exposes HTTP endpoints for controlling the trading bot's
// run/pause/stop state. All endpoints are guarded by RequireAPIKey.
type BotHandlers struct {
	controller *bot.Controller
	apiKey     string
}

// NewBotHandlers returns a BotHandlers that delegates state changes to
// controller and authenticates requests against apiKey.
func NewBotHandlers(controller *bot.Controller, apiKey string) *BotHandlers {
	return &BotHandlers{controller: controller, apiKey: apiKey}
}

// RequireAPIKey is middleware that enforces a Bearer token matching apiKey.
// Requests without a valid Authorization header are rejected with 401.
func (h *BotHandlers) RequireAPIKey(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(h.apiKey)) != 1 {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "unauthorized"})
			return
		}
		next(w, r)
	}
}

type statusResponse struct {
	Status string `json:"status"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// Start transitions the bot to running. Returns 409 if already running.
func (h *BotHandlers) Start(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	if err := h.controller.Start(); err != nil {
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{Status: h.controller.State().String()})
}

// Pause halts signal evaluation while keeping the stream connection alive.
// Returns 409 unless the bot is currently running.
func (h *BotHandlers) Pause(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	if err := h.controller.Pause(); err != nil {
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{Status: h.controller.State().String()})
}

// Stop transitions the bot to stopped from any non-stopped state.
// Returns 409 if already stopped.
func (h *BotHandlers) Stop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	if err := h.controller.Stop(); err != nil {
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{Status: h.controller.State().String()})
}

// Status returns the bot's current operational state as JSON.
func (h *BotHandlers) Status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, statusResponse{Status: h.controller.State().String()})
}
