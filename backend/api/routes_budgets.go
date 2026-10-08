package api

import "net/http"

// registerBudgetRoutes registers the /budgets routes. Owned by the budgets task; wrap
// protected handlers with h.requireAuth and read the caller with UserIDFrom.
func (h *Handler) registerBudgetRoutes(mux *http.ServeMux) {}
