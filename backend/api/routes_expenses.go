package api

import "net/http"

// registerExpenseRoutes registers the /expenses routes. Owned by the expenses task; wrap
// protected handlers with h.requireAuth and read the caller with UserIDFrom.
func (h *Handler) registerExpenseRoutes(mux *http.ServeMux) {}
