package api

import "net/http"

// registerExpenseRoutes registers the /expenses routes; all are behind auth.
func (h *Handler) registerExpenseRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /expenses", h.requireAuth(h.listExpenses))
	mux.HandleFunc("POST /expenses", h.requireAuth(h.createExpense))
	mux.HandleFunc("GET /expenses/{id}", h.requireAuth(h.getExpense))
	mux.HandleFunc("PUT /expenses/{id}", h.requireAuth(h.updateExpense))
	mux.HandleFunc("DELETE /expenses/{id}", h.requireAuth(h.deleteExpense))
}
