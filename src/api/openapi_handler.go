package api

import (
	_ "embed"
	"html/template"
	"net/http"
)

//go:embed openapi.json
var openAPISpec []byte

// ServeOpenAPISpec serves the OpenAPI JSON specification
// Per AI.md spec: JSON only, no YAML endpoint
func (h *Handler) ServeOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	w.Write(openAPISpec)
}

// Swagger UI HTML template with Dracula theme
const swaggerUIHTML = `<!DOCTYPE html>
<html>
<head>
  <title>Swagger UI - Search API</title>
  <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
  <style>
    /* Dracula Theme for Swagger UI */
    body {
      margin: 0;
      background: #282a36;
    }
    .swagger-ui {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    }
    .swagger-ui .topbar {
      background-color: #44475a;
      padding: 10px 0;
    }
    .swagger-ui .topbar .download-url-wrapper {
      display: flex;
      align-items: center;
    }
    .swagger-ui .topbar .download-url-wrapper .download-url-button {
      background: #50fa7b;
      color: #282a36;
      border: none;
      font-weight: bold;
    }
    .swagger-ui .topbar .download-url-wrapper input[type=text] {
      background: #44475a;
      color: #f8f8f2;
      border: 1px solid #6272a4;
    }
    .swagger-ui .info {
      margin: 20px 0;
    }
    .swagger-ui .info .title {
      color: #f8f8f2;
    }
    .swagger-ui .info .description p {
      color: #f8f8f2;
    }
    .swagger-ui .info .base-url {
      color: #8be9fd;
    }
    .swagger-ui .scheme-container {
      background: #44475a;
      box-shadow: none;
    }
    .swagger-ui .opblock-tag {
      color: #f8f8f2;
      border-bottom: 1px solid #44475a;
    }
    .swagger-ui .opblock-tag:hover {
      background: #44475a;
    }
    .swagger-ui .opblock {
      background: #44475a;
      border: 1px solid #6272a4;
      border-radius: 4px;
      margin-bottom: 10px;
    }
    .swagger-ui .opblock .opblock-summary {
      border-bottom: 1px solid #6272a4;
    }
    .swagger-ui .opblock .opblock-summary-method {
      font-weight: bold;
    }
    .swagger-ui .opblock.opblock-get {
      background: rgba(97, 175, 254, 0.1);
      border-color: #61affe;
    }
    .swagger-ui .opblock.opblock-get .opblock-summary-method {
      background: #61affe;
    }
    .swagger-ui .opblock.opblock-post {
      background: rgba(73, 204, 144, 0.1);
      border-color: #49cc90;
    }
    .swagger-ui .opblock.opblock-post .opblock-summary-method {
      background: #49cc90;
    }
    .swagger-ui .opblock.opblock-put {
      background: rgba(252, 161, 48, 0.1);
      border-color: #fca130;
    }
    .swagger-ui .opblock.opblock-put .opblock-summary-method {
      background: #fca130;
    }
    .swagger-ui .opblock.opblock-delete {
      background: rgba(249, 62, 62, 0.1);
      border-color: #f93e3e;
    }
    .swagger-ui .opblock.opblock-delete .opblock-summary-method {
      background: #f93e3e;
    }
    .swagger-ui .opblock .opblock-summary-path {
      color: #f8f8f2;
    }
    .swagger-ui .opblock .opblock-summary-description {
      color: #6272a4;
    }
    .swagger-ui .opblock-body {
      background: #282a36;
    }
    .swagger-ui .opblock-section-header {
      background: #44475a;
      box-shadow: none;
    }
    .swagger-ui .opblock-section-header h4 {
      color: #f8f8f2;
    }
    .swagger-ui .tab li {
      color: #f8f8f2;
    }
    .swagger-ui .tab li.active {
      color: #50fa7b;
    }
    .swagger-ui table thead tr th {
      color: #f8f8f2;
      border-bottom: 1px solid #6272a4;
    }
    .swagger-ui table tbody tr td {
      color: #f8f8f2;
      border-bottom: 1px solid #44475a;
    }
    .swagger-ui .parameter__name {
      color: #8be9fd;
    }
    .swagger-ui .parameter__type {
      color: #bd93f9;
    }
    .swagger-ui .parameter__in {
      color: #6272a4;
    }
    .swagger-ui .parameters-col_description p {
      color: #f8f8f2;
    }
    .swagger-ui .model-title {
      color: #f8f8f2;
    }
    .swagger-ui .model {
      color: #f8f8f2;
    }
    .swagger-ui .model-toggle::after {
      background: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 20 20'%3E%3Cpath fill='%23f8f8f2' d='M13.25 10L6.109 2.58c-.268-.27-.268-.707 0-.979.268-.27.701-.27.969 0l7.83 7.908c.268.271.268.709 0 .979l-7.83 7.908c-.268.27-.701.27-.969 0-.268-.269-.268-.707 0-.979L13.25 10z'/%3E%3C/svg%3E") center center no-repeat;
    }
    .swagger-ui .prop-type {
      color: #bd93f9;
    }
    .swagger-ui .prop-format {
      color: #6272a4;
    }
    .swagger-ui .response-col_status {
      color: #50fa7b;
    }
    .swagger-ui .response-col_description {
      color: #f8f8f2;
    }
    .swagger-ui .responses-inner h4, .swagger-ui .responses-inner h5 {
      color: #f8f8f2;
    }
    .swagger-ui .btn {
      background: #44475a;
      color: #f8f8f2;
      border: 1px solid #6272a4;
    }
    .swagger-ui .btn:hover {
      background: #6272a4;
    }
    .swagger-ui .btn.execute {
      background: #50fa7b;
      color: #282a36;
      border-color: #50fa7b;
    }
    .swagger-ui .btn.cancel {
      background: #ff5555;
      color: #f8f8f2;
      border-color: #ff5555;
    }
    .swagger-ui select {
      background: #44475a;
      color: #f8f8f2;
      border: 1px solid #6272a4;
    }
    .swagger-ui input[type=text], .swagger-ui textarea {
      background: #44475a;
      color: #f8f8f2;
      border: 1px solid #6272a4;
    }
    .swagger-ui .highlight-code {
      background: #282a36;
    }
    .swagger-ui .highlight-code > .microlight {
      background: #282a36;
      color: #f8f8f2;
    }
    .swagger-ui .response .response-col_status {
      color: #50fa7b;
    }
    .swagger-ui .loading-container .loading::before {
      border-color: #6272a4;
      border-top-color: #50fa7b;
    }
    .swagger-ui section.models {
      border: 1px solid #6272a4;
      border-radius: 4px;
    }
    .swagger-ui section.models h4 {
      color: #f8f8f2;
    }
    .swagger-ui section.models .model-container {
      background: #44475a;
      border-radius: 4px;
      margin: 5px 0;
    }
    .swagger-ui .model-box {
      background: #44475a;
    }
    .swagger-ui .servers > label {
      color: #f8f8f2;
    }
    .swagger-ui .servers > label select {
      background: #44475a;
      color: #f8f8f2;
      border: 1px solid #6272a4;
    }
    /* JSON syntax highlighting */
    .swagger-ui .microlight .hljs-attr {
      color: #50fa7b;
    }
    .swagger-ui .microlight .hljs-string {
      color: #f1fa8c;
    }
    .swagger-ui .microlight .hljs-number {
      color: #bd93f9;
    }
    .swagger-ui .microlight .hljs-literal {
      color: #bd93f9;
    }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-standalone-preset.js"></script>
  <script>
    window.onload = function() {
      window.ui = SwaggerUIBundle({
        url: "{{.SpecURL}}",
        dom_id: '#swagger-ui',
        deepLinking: true,
        presets: [
          SwaggerUIBundle.presets.apis,
          SwaggerUIStandalonePreset
        ],
        plugins: [
          SwaggerUIBundle.plugins.DownloadUrl
        ],
        layout: "StandaloneLayout",
        defaultModelsExpandDepth: 1,
        defaultModelExpandDepth: 1,
        displayRequestDuration: true,
        filter: true,
        showExtensions: true,
        showCommonExtensions: true,
        tryItOutEnabled: true
      });
    };
  </script>
</body>
</html>`

// SwaggerUIData holds template data for Swagger UI
type SwaggerUIData struct {
	SpecURL string
}

// ServeSwaggerUI serves the Swagger UI HTML page
func (h *Handler) ServeSwaggerUI(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.New("swagger").Parse(swaggerUIHTML)
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	// Determine the spec URL based on request
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if fwdProto := r.Header.Get("X-Forwarded-Proto"); fwdProto != "" {
		scheme = fwdProto
	}

	specURL := scheme + "://" + r.Host + "/openapi.json"

	data := SwaggerUIData{
		SpecURL: specURL,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "Template execution error", http.StatusInternalServerError)
		return
	}
}

// RegisterOpenAPIRoutes registers the OpenAPI/Swagger routes
// Per AI.md spec: /openapi (Swagger UI), /openapi.json (JSON only, no YAML)
func (h *Handler) RegisterOpenAPIRoutes(mux *http.ServeMux) {
	// OpenAPI JSON spec (JSON only per spec)
	mux.HandleFunc("/openapi.json", h.ServeOpenAPISpec)

	// Swagger UI
	mux.HandleFunc("/openapi", h.ServeSwaggerUI)
}
