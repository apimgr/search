package swagger

// getSwaggerThemeCSS returns CSS for Swagger UI theming
// Per TEMPLATE.md PART 19: Swagger & GraphQL Theming (NON-NEGOTIABLE)
// Swagger must match project-wide theme system (light/dark/auto)
func getSwaggerThemeCSS(theme string) string {
	if theme == "light" {
		return swaggerLightTheme
	}
	return swaggerDarkTheme // Default to dark
}

// swaggerDarkTheme provides dark theme CSS for Swagger UI
// Per TEMPLATE.md PART 19: Dark theme colors
const swaggerDarkTheme = `
/* Swagger UI - Dark Theme */
/* Per TEMPLATE.md PART 19: Swagger & GraphQL Theming */

.swagger-ui {
	background: #282a36;
	color: #f8f8f2;
}

.swagger-ui .topbar {
	background: #1e1f29;
	border-bottom: 1px solid #44475a;
}

.swagger-ui .topbar .download-url-wrapper .select-label {
	color: #f8f8f2;
}

.swagger-ui .info .title,
.swagger-ui .opblock-tag {
	color: #f8f8f2;
}

.swagger-ui .info .title small {
	background: #44475a;
	color: #f8f8f2;
}

.swagger-ui .opblock.opblock-get {
	background: rgba(139, 233, 253, 0.1);
	border-color: #8be9fd;
}

.swagger-ui .opblock.opblock-get .opblock-summary-method {
	background: #8be9fd;
	color: #282a36;
}

.swagger-ui .opblock.opblock-post {
	background: rgba(80, 250, 123, 0.1);
	border-color: #50fa7b;
}

.swagger-ui .opblock.opblock-post .opblock-summary-method {
	background: #50fa7b;
	color: #282a36;
}

.swagger-ui .opblock.opblock-put {
	background: rgba(255, 184, 108, 0.1);
	border-color: #ffb86c;
}

.swagger-ui .opblock.opblock-put .opblock-summary-method {
	background: #ffb86c;
	color: #282a36;
}

.swagger-ui .opblock.opblock-delete {
	background: rgba(255, 85, 85, 0.1);
	border-color: #ff5555;
}

.swagger-ui .opblock.opblock-delete .opblock-summary-method {
	background: #ff5555;
	color: #f8f8f2;
}

.swagger-ui .opblock.opblock-patch {
	background: rgba(189, 147, 249, 0.1);
	border-color: #bd93f9;
}

.swagger-ui .opblock.opblock-patch .opblock-summary-method {
	background: #bd93f9;
	color: #282a36;
}

.swagger-ui input,
.swagger-ui textarea,
.swagger-ui select {
	background: #44475a;
	color: #f8f8f2;
	border: 1px solid #6272a4;
}

.swagger-ui input:focus,
.swagger-ui textarea:focus,
.swagger-ui select:focus {
	border-color: #bd93f9;
	outline: none;
}

.swagger-ui .btn {
	background: #6272a4;
	color: #f8f8f2;
	border: none;
}

.swagger-ui .btn:hover {
	background: #bd93f9;
}

.swagger-ui .btn.execute {
	background: #50fa7b;
	color: #282a36;
}

.swagger-ui .btn.execute:hover {
	background: #8be9fd;
}

.swagger-ui .scheme-container {
	background: #44475a;
	border: 1px solid #6272a4;
}

.swagger-ui .model-box {
	background: #44475a;
	color: #f8f8f2;
}

.swagger-ui section.models {
	border-color: #6272a4;
}

.swagger-ui .model {
	color: #f8f8f2;
}

.swagger-ui .model-title {
	color: #bd93f9;
}

.swagger-ui table thead tr th,
.swagger-ui table thead tr td {
	color: #f8f8f2;
	border-bottom-color: #6272a4;
}

.swagger-ui table tbody tr td {
	color: #f8f8f2;
	border-color: #6272a4;
}

.swagger-ui .parameter__name {
	color: #8be9fd;
}

.swagger-ui .parameter__type {
	color: #50fa7b;
}

.swagger-ui .response-col_status {
	color: #bd93f9;
}

.swagger-ui .response-col_description {
	color: #f8f8f2;
}
`

// swaggerLightTheme provides light theme CSS for Swagger UI
// Per TEMPLATE.md PART 19: Light theme colors
const swaggerLightTheme = `
/* Swagger UI - Light Theme */
/* Per TEMPLATE.md PART 19: Swagger & GraphQL Theming */

.swagger-ui {
	background: #ffffff;
	color: #1a1a1a;
}

.swagger-ui .topbar {
	background: #f5f5f5;
	border-bottom: 1px solid #e0e0e0;
}

.swagger-ui .info .title,
.swagger-ui .opblock-tag {
	color: #1a1a1a;
}

.swagger-ui .info .title small {
	background: #e0e0e0;
	color: #1a1a1a;
}

.swagger-ui .opblock.opblock-get {
	background: rgba(0, 102, 204, 0.05);
	border-color: #0066cc;
}

.swagger-ui .opblock.opblock-get .opblock-summary-method {
	background: #0066cc;
	color: #ffffff;
}

.swagger-ui .opblock.opblock-post {
	background: rgba(0, 128, 0, 0.05);
	border-color: #008000;
}

.swagger-ui .opblock.opblock-post .opblock-summary-method {
	background: #008000;
	color: #ffffff;
}

.swagger-ui .opblock.opblock-put {
	background: rgba(255, 140, 0, 0.05);
	border-color: #ff8c00;
}

.swagger-ui .opblock.opblock-put .opblock-summary-method {
	background: #ff8c00;
	color: #ffffff;
}

.swagger-ui .opblock.opblock-delete {
	background: rgba(204, 0, 0, 0.05);
	border-color: #cc0000;
}

.swagger-ui .opblock.opblock-delete .opblock-summary-method {
	background: #cc0000;
	color: #ffffff;
}

.swagger-ui .opblock.opblock-patch {
	background: rgba(102, 0, 204, 0.05);
	border-color: #6600cc;
}

.swagger-ui .opblock.opblock-patch .opblock-summary-method {
	background: #6600cc;
	color: #ffffff;
}

.swagger-ui input,
.swagger-ui textarea,
.swagger-ui select {
	background: #ffffff;
	color: #1a1a1a;
	border: 1px solid #cccccc;
}

.swagger-ui input:focus,
.swagger-ui textarea:focus,
.swagger-ui select:focus {
	border-color: #0066cc;
	outline: none;
}

.swagger-ui .btn {
	background: #0066cc;
	color: #ffffff;
	border: none;
}

.swagger-ui .btn:hover {
	background: #0052a3;
}

.swagger-ui .btn.execute {
	background: #008000;
	color: #ffffff;
}

.swagger-ui .btn.execute:hover {
	background: #006600;
}

.swagger-ui .scheme-container {
	background: #f5f5f5;
	border: 1px solid #cccccc;
}

.swagger-ui .model-box {
	background: #f5f5f5;
	color: #1a1a1a;
}

.swagger-ui section.models {
	border-color: #cccccc;
}

.swagger-ui .model {
	color: #1a1a1a;
}

.swagger-ui .model-title {
	color: #0066cc;
}

.swagger-ui table thead tr th,
.swagger-ui table thead tr td {
	color: #1a1a1a;
	border-bottom-color: #cccccc;
}

.swagger-ui table tbody tr td {
	color: #1a1a1a;
	border-color: #cccccc;
}

.swagger-ui .parameter__name {
	color: #0066cc;
}

.swagger-ui .parameter__type {
	color: #008000;
}

.swagger-ui .response-col_status {
	color: #6600cc;
}

.swagger-ui .response-col_description {
	color: #1a1a1a;
}
`
