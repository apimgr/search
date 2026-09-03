package graphql

// getGraphiQLThemeCSS returns CSS for GraphiQL theming
// Per AI.md PART 19: Swagger & GraphQL Theming (NON-NEGOTIABLE)
// GraphQL must match project-wide theme system (light/dark/auto)
func getGraphiQLThemeCSS(theme string) string {
	if theme == "light" {
		return graphiqlLightTheme
	}
	// Default to dark
	return graphiqlDarkTheme
}

// graphiqlDarkTheme provides dark theme CSS for GraphiQL
// Per AI.md PART 19: Dark theme colors
const graphiqlDarkTheme = `
/* GraphiQL - Dark Theme */
/* Per AI.md PART 19: Swagger & GraphQL Theming */

#graphiql {
	height: 100vh;
}

.graphiql-container {
	background: #282a36;
	color: #f8f8f2;
}

.graphiql-container .topBar {
	background: #1e1f29;
	border-bottom: 1px solid #44475a;
}

.graphiql-container .title {
	color: #f8f8f2;
}

.graphiql-container .CodeMirror {
	background: #282a36;
	color: #f8f8f2;
}

.graphiql-container .CodeMirror-gutters {
	background: #1e1f29;
	border-right: 1px solid #44475a;
}

.graphiql-container .CodeMirror-linenumber {
	color: #6272a4;
}

.graphiql-container .CodeMirror-cursor {
	border-left-color: #f8f8f2;
}

.graphiql-container .CodeMirror-selected {
	background: #44475a;
}

.graphiql-container .result-window {
	background: #282a36;
	color: #f8f8f2;
}

.graphiql-container .execute-button {
	background: #50fa7b;
	color: #282a36;
	border: none;
}

.graphiql-container .execute-button:hover {
	background: #8be9fd;
}

.graphiql-container .execute-button:active {
	background: #50fa7b;
}

.graphiql-container .toolbar-button {
	background: #44475a;
	color: #f8f8f2;
	border: 1px solid #6272a4;
}

.graphiql-container .toolbar-button:hover {
	background: #6272a4;
}

.graphiql-container input,
.graphiql-container select {
	background: #44475a;
	color: #f8f8f2;
	border: 1px solid #6272a4;
}

.graphiql-container input:focus,
.graphiql-container select:focus {
	border-color: #bd93f9;
	outline: none;
}

/* Syntax highlighting for GraphQL queries */
.cm-s-graphiql .cm-property {
	color: #8be9fd;
}

.cm-s-graphiql .cm-keyword {
	color: #ff79c6;
}

.cm-s-graphiql .cm-def {
	color: #50fa7b;
}

.cm-s-graphiql .cm-variable {
	color: #f8f8f2;
}

.cm-s-graphiql .cm-string {
	color: #f1fa8c;
}

.cm-s-graphiql .cm-number {
	color: #bd93f9;
}

.cm-s-graphiql .cm-comment {
	color: #6272a4;
}

.cm-s-graphiql .cm-punctuation {
	color: #f8f8f2;
}

.cm-s-graphiql .cm-attribute {
	color: #50fa7b;
}

.cm-s-graphiql .cm-type {
	color: #8be9fd;
}

/* Response pane */
.graphiql-container .result-window .CodeMirror-scroll {
	background: #282a36;
}

/* Documentation explorer */
.graphiql-container .doc-explorer {
	background: #282a36;
	color: #f8f8f2;
	border-left: 1px solid #44475a;
}

.graphiql-container .doc-explorer-title {
	background: #1e1f29;
	color: #f8f8f2;
	border-bottom: 1px solid #44475a;
}

.graphiql-container .doc-type-description {
	color: #f8f8f2;
}

.graphiql-container .doc-category-title {
	color: #bd93f9;
}

.graphiql-container .field-name {
	color: #8be9fd;
}

.graphiql-container .type-name {
	color: #50fa7b;
}

.graphiql-container .arg-name {
	color: #ffb86c;
}

/* History pane */
.graphiql-container .history-contents {
	background: #282a36;
	color: #f8f8f2;
}

.graphiql-container .history-title {
	background: #1e1f29;
	color: #f8f8f2;
	border-bottom: 1px solid #44475a;
}
`

// graphiqlLightTheme provides light theme CSS for GraphiQL
// Per AI.md PART 19: Light theme colors (Unified Color Palette / GitHub-Light-based)
const graphiqlLightTheme = `
/* GraphiQL - Light Theme */
/* Per AI.md PART 19: Swagger & GraphQL Theming */

#graphiql {
	height: 100vh;
}

.graphiql-container {
	background: #ffffff;
	color: #1f2328;
}

.graphiql-container .topBar {
	background: #f6f8fa;
	border-bottom: 1px solid #d1d9e0;
}

.graphiql-container .title {
	color: #1f2328;
}

.graphiql-container .CodeMirror {
	background: #ffffff;
	color: #1f2328;
}

.graphiql-container .CodeMirror-gutters {
	background: #f6f8fa;
	border-right: 1px solid #d1d9e0;
}

.graphiql-container .CodeMirror-linenumber {
	color: #59636e;
}

.graphiql-container .CodeMirror-cursor {
	border-left-color: #1f2328;
}

.graphiql-container .CodeMirror-selected {
	background: #d1d9e0;
}

.graphiql-container .result-window {
	background: #ffffff;
	color: #1f2328;
}

.graphiql-container .execute-button {
	background: #1a7f37;
	color: #ffffff;
	border: none;
}

.graphiql-container .execute-button:hover {
	background: #156a2e;
}

.graphiql-container .toolbar-button {
	background: #f6f8fa;
	color: #1f2328;
	border: 1px solid #d1d9e0;
}

.graphiql-container .toolbar-button:hover {
	background: #d1d9e0;
}

.graphiql-container input,
.graphiql-container select {
	background: #ffffff;
	color: #1f2328;
	border: 1px solid #d1d9e0;
}

.graphiql-container input:focus,
.graphiql-container select:focus {
	border-color: #0969da;
	outline: none;
}

/* Syntax highlighting for GraphQL queries */
.cm-s-graphiql .cm-property {
	color: #0969da;
}

.cm-s-graphiql .cm-keyword {
	color: #8250df;
}

.cm-s-graphiql .cm-def {
	color: #1a7f37;
}

.cm-s-graphiql .cm-variable {
	color: #1f2328;
}

.cm-s-graphiql .cm-string {
	color: #9a6700;
}

.cm-s-graphiql .cm-number {
	color: #8250df;
}

.cm-s-graphiql .cm-comment {
	color: #59636e;
}

.cm-s-graphiql .cm-punctuation {
	color: #1f2328;
}

.cm-s-graphiql .cm-attribute {
	color: #1a7f37;
}

.cm-s-graphiql .cm-type {
	color: #0969da;
}

/* Response pane */
.graphiql-container .result-window .CodeMirror-scroll {
	background: #ffffff;
}

/* Documentation explorer */
.graphiql-container .doc-explorer {
	background: #ffffff;
	color: #1f2328;
	border-left: 1px solid #d1d9e0;
}

.graphiql-container .doc-explorer-title {
	background: #f6f8fa;
	color: #1f2328;
	border-bottom: 1px solid #d1d9e0;
}

.graphiql-container .doc-type-description {
	color: #1f2328;
}

.graphiql-container .doc-category-title {
	color: #8250df;
}

.graphiql-container .field-name {
	color: #0969da;
}

.graphiql-container .type-name {
	color: #1a7f37;
}

.graphiql-container .arg-name {
	color: #9a6700;
}

/* History pane */
.graphiql-container .history-contents {
	background: #ffffff;
	color: #1f2328;
}

.graphiql-container .history-title {
	background: #f6f8fa;
	color: #1f2328;
	border-bottom: 1px solid #d1d9e0;
}
`
