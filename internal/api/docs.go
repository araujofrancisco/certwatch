package api

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.yaml
var openapiSpec []byte

//go:embed web/templates/docs.html
var docsHTML string

func (h *Handler) RegisterDocsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/docs/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(openapiSpec)
	})

	mux.HandleFunc("GET /api/docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(docsHTML))
	})
}
