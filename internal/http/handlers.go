package http

import (
	"encoding/json"
	"net/http"

	"github.com/wu-piyaphon/outbound-api/internal/bot"
)

type BotHandlers struct {
	controller *bot.Controller
}

func NewBotHandlers(controller *bot.Controller) *BotHandlers {
	return &BotHandlers{controller: controller}
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

func (h *BotHandlers) Status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, statusResponse{Status: h.controller.State().String()})
}
