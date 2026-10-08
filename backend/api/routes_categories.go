package api

import "net/http"

// registerCategoryRoutes registers the /categories routes. Owned by the categories task; wrap
// protected handlers with h.requireAuth and read the caller with UserIDFrom.
func (h *Handler) registerCategoryRoutes(mux *http.ServeMux) {}
