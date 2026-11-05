package swagger

import "github.com/apimgr/search/src/common/theme"

func getSwaggerThemeCSS(mode string) string {
	switch mode {
	case "light":
		return swaggerLightTheme
	case "auto":
		return swaggerDarkTheme +
			"\n@media (prefers-color-scheme: light) {\n" +
			theme.CSSPaletteVariables(":root", theme.ThemePaletteLight) + "}\n"
	default:
		return swaggerDarkTheme
	}
}

var (
	swaggerDarkTheme  = theme.CSSPaletteVariables(":root", theme.ThemePaletteDark) + swaggerThemeCSS
	swaggerLightTheme = theme.CSSPaletteVariables(":root", theme.ThemePaletteLight) + swaggerThemeCSS
)

const swaggerThemeCSS = `
.swagger-ui { background: var(--palette-background); color: var(--palette-foreground); }
.swagger-ui .topbar { background: var(--palette-surface-alt); border-bottom: 1px solid var(--palette-border); }
.swagger-ui .topbar .download-url-wrapper .select-label,
.swagger-ui .info .title, .swagger-ui .opblock-tag { color: var(--palette-foreground); }
.swagger-ui .info .title small { background: var(--palette-surface); color: var(--palette-foreground); }
.swagger-ui .opblock { background: color-mix(in srgb, var(--palette-primary) 8%, transparent); }
.swagger-ui .opblock.opblock-get { border-color: var(--palette-info); }
.swagger-ui .opblock.opblock-get .opblock-summary-method { background: var(--palette-info); color: var(--palette-background); }
.swagger-ui .opblock.opblock-post { border-color: var(--palette-success); }
.swagger-ui .opblock.opblock-post .opblock-summary-method { background: var(--palette-success); color: var(--palette-background); }
.swagger-ui .opblock.opblock-put { border-color: var(--palette-warning); }
.swagger-ui .opblock.opblock-put .opblock-summary-method { background: var(--palette-warning); color: var(--palette-background); }
.swagger-ui .opblock.opblock-delete { border-color: var(--palette-error); }
.swagger-ui .opblock.opblock-delete .opblock-summary-method { background: var(--palette-error); color: var(--palette-foreground); }
.swagger-ui .opblock.opblock-patch { border-color: var(--palette-primary); }
.swagger-ui .opblock.opblock-patch .opblock-summary-method { background: var(--palette-primary); color: var(--palette-background); }
.swagger-ui input, .swagger-ui textarea, .swagger-ui select { background: var(--palette-surface); color: var(--palette-foreground); border: 1px solid var(--palette-border); }
.swagger-ui input:focus, .swagger-ui textarea:focus, .swagger-ui select:focus { border-color: var(--palette-primary); outline: none; }
.swagger-ui .btn { background: var(--palette-muted); color: var(--palette-foreground); border: none; }
.swagger-ui .btn:hover { background: var(--palette-primary); }
.swagger-ui .btn.execute { background: var(--palette-success); color: var(--palette-background); }
.swagger-ui .btn.execute:hover { background: var(--palette-info); }
.swagger-ui .scheme-container, .swagger-ui .model-box { background: var(--palette-surface); border: 1px solid var(--palette-border); color: var(--palette-foreground); }
.swagger-ui section.models, .swagger-ui table tbody tr td { border-color: var(--palette-border); }
.swagger-ui .model, .swagger-ui table tbody tr td, .swagger-ui .response-col_description { color: var(--palette-foreground); }
.swagger-ui .model-title, .swagger-ui .response-col_status { color: var(--palette-primary); }
.swagger-ui table thead tr th, .swagger-ui table thead tr td { color: var(--palette-foreground); border-bottom-color: var(--palette-border); }
.swagger-ui .parameter__name { color: var(--palette-info); }
.swagger-ui .parameter__type { color: var(--palette-success); }
`
