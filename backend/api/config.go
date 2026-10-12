package api

import (
	"net/http"

	"budgetcontrol/httpx"
)

// registerConfigRoutes registers the public GET /config route. It needs no
// session and does not touch the database.
func (h *Handler) registerConfigRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /config", h.config)
}

// config publishes limits the frontend needs, so they have one owner.
func (h *Handler) config(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]int64{"maxAmount": maxAmount})
}
