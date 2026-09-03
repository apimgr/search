package swagger

// getSwaggerThemeCSS returns CSS for Swagger UI theming
// Per AI.md PART 19: Swagger & GraphQL Theming (NON-NEGOTIABLE)
// Swagger must match project-wide theme system (light/dark/auto)
func getSwaggerThemeCSS(theme string) string {
	if theme == "light" {
		return swaggerLightTheme
	}
	// Default to dark
	return swaggerDarkTheme
}

// swaggerDarkTheme provides dark theme CSS for Swagger UI
// Per AI.md PART 19: Dark theme colors
const swaggerDarkTheme = `
/* Swagger UI - Dark Theme */
/* Per AI.md PART 19: Swagger & GraphQL Theming */

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
// Per AI.md PART 19: Light theme colors (Unified Color Palette / GitHub-Light-based)
const swaggerLightTheme = `
/* Swagger UI - Light Theme */
/* Per AI.md PART 19: Swagger & GraphQL Theming */

.swagger-ui {
	background: #ffffff;
	color: #1f2328;
}

.swagger-ui .topbar {
	background: #f6f8fa;
	border-bottom: 1px solid #d1d9e0;
}

.swagger-ui .info .title,
.swagger-ui .opblock-tag {
	color: #1f2328;
}

.swagger-ui .info .title small {
	background: #d1d9e0;
	color: #1f2328;
}

.swagger-ui .opblock.opblock-get {
	background: rgba(9, 105, 218, 0.05);
	border-color: #0969da;
}

.swagger-ui .opblock.opblock-get .opblock-summary-method {
	background: #0969da;
	color: #ffffff;
}

.swagger-ui .opblock.opblock-post {
	background: rgba(26, 127, 55, 0.05);
	border-color: #1a7f37;
}

.swagger-ui .opblock.opblock-post .opblock-summary-method {
	background: #1a7f37;
	color: #ffffff;
}

.swagger-ui .opblock.opblock-put {
	background: rgba(154, 103, 0, 0.05);
	border-color: #9a6700;
}

.swagger-ui .opblock.opblock-put .opblock-summary-method {
	background: #9a6700;
	color: #ffffff;
}

.swagger-ui .opblock.opblock-delete {
	background: rgba(209, 36, 47, 0.05);
	border-color: #d1242f;
}

.swagger-ui .opblock.opblock-delete .opblock-summary-method {
	background: #d1242f;
	color: #ffffff;
}

.swagger-ui .opblock.opblock-patch {
	background: rgba(130, 80, 223, 0.05);
	border-color: #8250df;
}

.swagger-ui .opblock.opblock-patch .opblock-summary-method {
	background: #8250df;
	color: #ffffff;
}

.swagger-ui input,
.swagger-ui textarea,
.swagger-ui select {
	background: #ffffff;
	color: #1f2328;
	border: 1px solid #d1d9e0;
}

.swagger-ui input:focus,
.swagger-ui textarea:focus,
.swagger-ui select:focus {
	border-color: #0969da;
	outline: none;
}

.swagger-ui .btn {
	background: #0969da;
	color: #ffffff;
	border: none;
}

.swagger-ui .btn:hover {
	background: #0757ba;
}

.swagger-ui .btn.execute {
	background: #1a7f37;
	color: #ffffff;
}

.swagger-ui .btn.execute:hover {
	background: #156a2e;
}

.swagger-ui .scheme-container {
	background: #f6f8fa;
	border: 1px solid #d1d9e0;
}

.swagger-ui .model-box {
	background: #f6f8fa;
	color: #1f2328;
}

.swagger-ui section.models {
	border-color: #d1d9e0;
}

.swagger-ui .model {
	color: #1f2328;
}

.swagger-ui .model-title {
	color: #0969da;
}

.swagger-ui table thead tr th,
.swagger-ui table thead tr td {
	color: #1f2328;
	border-bottom-color: #d1d9e0;
}

.swagger-ui table tbody tr td {
	color: #1f2328;
	border-color: #d1d9e0;
}

.swagger-ui .parameter__name {
	color: #0969da;
}

.swagger-ui .parameter__type {
	color: #1a7f37;
}

.swagger-ui .response-col_status {
	color: #8250df;
}

.swagger-ui .response-col_description {
	color: #1f2328;
}
`
