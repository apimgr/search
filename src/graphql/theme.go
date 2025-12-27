package graphql

// getGraphiQLThemeCSS returns CSS for GraphiQL theming
// Per TEMPLATE.md PART 19: Swagger & GraphQL Theming (NON-NEGOTIABLE)
// GraphQL must match project-wide theme system (light/dark/auto)
func getGraphiQLThemeCSS(theme string) string {
	if theme == "light" {
		return graphiqlLightTheme
	}
	return graphiqlDarkTheme // Default to dark
}

// graphiqlDarkTheme provides dark theme CSS for GraphiQL
// Per TEMPLATE.md PART 19: Dark theme colors
const graphiqlDarkTheme = `
/* GraphiQL - Dark Theme */
/* Per TEMPLATE.md PART 19: Swagger & GraphQL Theming */

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
// Per TEMPLATE.md PART 19: Light theme colors
const graphiqlLightTheme = `
/* GraphiQL - Light Theme */
/* Per TEMPLATE.md PART 19: Swagger & GraphQL Theming */

.graphiql-container {
	background: #ffffff;
	color: #1a1a1a;
}

.graphiql-container .topBar {
	background: #f5f5f5;
	border-bottom: 1px solid #e0e0e0;
}

.graphiql-container .title {
	color: #1a1a1a;
}

.graphiql-container .CodeMirror {
	background: #ffffff;
	color: #1a1a1a;
}

.graphiql-container .CodeMirror-gutters {
	background: #f5f5f5;
	border-right: 1px solid #e0e0e0;
}

.graphiql-container .CodeMirror-linenumber {
	color: #666666;
}

.graphiql-container .CodeMirror-cursor {
	border-left-color: #1a1a1a;
}

.graphiql-container .CodeMirror-selected {
	background: #e0e0e0;
}

.graphiql-container .result-window {
	background: #ffffff;
	color: #1a1a1a;
}

.graphiql-container .execute-button {
	background: #008000;
	color: #ffffff;
	border: none;
}

.graphiql-container .execute-button:hover {
	background: #006600;
}

.graphiql-container .toolbar-button {
	background: #f5f5f5;
	color: #1a1a1a;
	border: 1px solid #cccccc;
}

.graphiql-container .toolbar-button:hover {
	background: #e0e0e0;
}

.graphiql-container input,
.graphiql-container select {
	background: #ffffff;
	color: #1a1a1a;
	border: 1px solid #cccccc;
}

.graphiql-container input:focus,
.graphiql-container select:focus {
	border-color: #0066cc;
	outline: none;
}

/* Syntax highlighting for GraphQL queries */
.cm-s-graphiql .cm-property {
	color: #0066cc;
}

.cm-s-graphiql .cm-keyword {
	color: #6600cc;
}

.cm-s-graphiql .cm-def {
	color: #008000;
}

.cm-s-graphiql .cm-variable {
	color: #1a1a1a;
}

.cm-s-graphiql .cm-string {
	color: #cc6600;
}

.cm-s-graphiql .cm-number {
	color: #6600cc;
}

.cm-s-graphiql .cm-comment {
	color: #666666;
}

.cm-s-graphiql .cm-punctuation {
	color: #1a1a1a;
}

.cm-s-graphiql .cm-attribute {
	color: #008000;
}

.cm-s-graphiql .cm-type {
	color: #0066cc;
}

/* Response pane */
.graphiql-container .result-window .CodeMirror-scroll {
	background: #ffffff;
}

/* Documentation explorer */
.graphiql-container .doc-explorer {
	background: #ffffff;
	color: #1a1a1a;
	border-left: 1px solid #e0e0e0;
}

.graphiql-container .doc-explorer-title {
	background: #f5f5f5;
	color: #1a1a1a;
	border-bottom: 1px solid #e0e0e0;
}

.graphiql-container .doc-type-description {
	color: #1a1a1a;
}

.graphiql-container .doc-category-title {
	color: #6600cc;
}

.graphiql-container .field-name {
	color: #0066cc;
}

.graphiql-container .type-name {
	color: #008000;
}

.graphiql-container .arg-name {
	color: #ff8c00;
}

/* History pane */
.graphiql-container .history-contents {
	background: #ffffff;
	color: #1a1a1a;
}

.graphiql-container .history-title {
	background: #f5f5f5;
	color: #1a1a1a;
	border-bottom: 1px solid #e0e0e0;
}
`
