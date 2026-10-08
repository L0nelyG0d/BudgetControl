package api

import "net/http"

// registerBudgetRoutes registers the /budgets routes; all are behind auth.
func (h *Handler) registerBudgetRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /budgets", h.requireAuth(h.listBudgets))
	mux.HandleFunc("PUT /budgets", h.requireAuth(h.setBudget))
	mux.HandleFunc("DELETE /budgets", h.requireAuth(h.deleteBudget))
}
