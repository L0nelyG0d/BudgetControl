package api

import "net/http"

// registerCategoryRoutes registers the /categories routes; every one needs a session.
func (h *Handler) registerCategoryRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /categories", h.requireAuth(h.listCategories))
	mux.HandleFunc("POST /categories", h.requireAuth(h.createCategory))
	mux.HandleFunc("PUT /categories/{id}", h.requireAuth(h.updateCategory))
	mux.HandleFunc("DELETE /categories/{id}", h.requireAuth(h.deleteCategory))
}
