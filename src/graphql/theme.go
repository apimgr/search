package graphql

import "github.com/apimgr/search/src/common/theme"

func getGraphiQLThemeCSS(mode string) string {
	switch mode {
	case "light":
		return graphiqlLightTheme
	case "auto":
		return graphiqlDarkTheme +
			"\n@media (prefers-color-scheme: light) {\n" +
			theme.CSSPaletteVariables(":root", theme.ThemePaletteLight) + "}\n"
	default:
		return graphiqlDarkTheme
	}
}

var (
	graphiqlDarkTheme  = theme.CSSPaletteVariables(":root", theme.ThemePaletteDark) + graphiqlThemeCSS
	graphiqlLightTheme = theme.CSSPaletteVariables(":root", theme.ThemePaletteLight) + graphiqlThemeCSS
)

const graphiqlThemeCSS = `
#graphiql { height: 100vh; }
.graphiql-container { background: var(--palette-background); color: var(--palette-foreground); }
.graphiql-container .topBar, .graphiql-container .CodeMirror-gutters,
.graphiql-container .doc-explorer-title, .graphiql-container .history-title {
  background: var(--palette-surface-alt); border-bottom: 1px solid var(--palette-border);
}
.graphiql-container .title, .graphiql-container .CodeMirror,
.graphiql-container .result-window, .graphiql-container .history-contents,
.graphiql-container .doc-explorer, .graphiql-container .doc-type-description {
  background: var(--palette-background); color: var(--palette-foreground);
}
.graphiql-container .CodeMirror-linenumber, .cm-s-graphiql .cm-comment { color: var(--palette-muted); }
.graphiql-container .CodeMirror-cursor { border-left-color: var(--palette-foreground); }
.graphiql-container .CodeMirror-selected { background: var(--palette-surface); }
.graphiql-container .execute-button { background: var(--palette-success); color: var(--palette-background); border: none; }
.graphiql-container .execute-button:hover { background: var(--palette-info); }
.graphiql-container .toolbar-button, .graphiql-container input, .graphiql-container select {
  background: var(--palette-surface); color: var(--palette-foreground); border: 1px solid var(--palette-border);
}
.graphiql-container .toolbar-button:hover { background: var(--palette-border); }
.graphiql-container input:focus, .graphiql-container select:focus { border-color: var(--palette-primary); outline: none; }
.cm-s-graphiql .cm-property, .cm-s-graphiql .cm-type, .graphiql-container .field-name { color: var(--palette-info); }
.cm-s-graphiql .cm-keyword, .graphiql-container .doc-category-title { color: var(--palette-primary); }
.cm-s-graphiql .cm-def, .cm-s-graphiql .cm-attribute, .graphiql-container .type-name { color: var(--palette-success); }
.cm-s-graphiql .cm-variable, .cm-s-graphiql .cm-punctuation { color: var(--palette-foreground); }
.cm-s-graphiql .cm-string, .graphiql-container .arg-name { color: var(--palette-warning); }
.cm-s-graphiql .cm-number { color: var(--palette-accent); }
.graphiql-container .result-window .CodeMirror-scroll { background: var(--palette-background); }
.graphiql-container .doc-explorer { border-left: 1px solid var(--palette-border); }
.graphiql-container .history-title { color: var(--palette-foreground); }
`
