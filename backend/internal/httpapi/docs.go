package httpapi

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.json
var openAPI []byte

func registerDocsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(openAPI)
	})
	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>AmbatuMove API</title><link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css"></head><body><div id="swagger-ui"></div><script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js"></script><script>SwaggerUIBundle({url:"/openapi.json",dom_id:"#swagger-ui",deepLinking:true,displayRequestDuration:true})</script><noscript><a href="/openapi.json">OpenAPI 3.1 JSON</a></noscript></body></html>`))
	})
}
