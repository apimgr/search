package direct

import (
	"html"
	"strings"
	"unicode"
	"unicode/utf8"
)

var syntaxKeywords = map[string]map[string]struct{}{
	"javascript": keywordSet("async await break case catch class const continue debugger default delete do else export extends finally for from function if import in instanceof let new of return static super switch this throw try typeof var void while with yield", "true false null undefined"),
	"typescript": keywordSet("abstract any as async await boolean break case catch class const constructor continue debugger declare default delete do else enum export extends finally for from function implements import in infer instanceof interface is keyof let module namespace never new number object of package private protected public readonly require return static string super switch symbol this throw type typeof var void while with yield", "true false null undefined"),
	"php":        keywordSet("abstract and array as break callable case catch class clone const continue declare default die do echo else elseif empty endfor endforeach endif endswitch endwhile eval exit extends final finally fn for foreach function global goto if implements include include_once instanceof insteadof interface isset list namespace new or print private protected public require require_once return static switch throw trait try unset use var while xor yield", "true false null self parent"),
	"python":     keywordSet("and as assert async await break class continue def del elif else except finally for from global if import in is lambda nonlocal not or pass raise return try while with yield", "False None True"),
	"sql":        keywordSet("add all alter analyze and any as asc begin between by case cascade cast check column commit constraint create cross current_date current_time database day declare default delete desc distinct drop else end escape except exists explain false fetch for foreign from full grant group having if in index inner insert intersect into is join key left like limit natural not null offset on or order outer over partition primary procedure references returning right rollback row select set table then to true union unique update use using values view when where with", "null true false"),
	"json":       keywordSet("true false null", "true false null"),
	"css":        keywordSet("important media supports keyframes font-face import namespace page", ""),
	"scss":       keywordSet("important media supports keyframes font-face import namespace page include mixin extend function return if else each for while", ""),
	"less":       keywordSet("important media supports keyframes font-face import namespace page", ""),
}

func keywordSet(groups ...string) map[string]struct{} {
	keywords := make(map[string]struct{})
	for _, group := range groups {
		for _, keyword := range strings.Fields(group) {
			keywords[keyword] = struct{}{}
		}
	}
	return keywords
}

func highlightFormattedCode(source, language string) string {
	source = strings.ToValidUTF8(source, "\uFFFD")
	language = strings.ToLower(language)
	switch language {
	case "js":
		language = "javascript"
	case "ts":
		language = "typescript"
	case "xml":
		language = "html"
	}
	var output strings.Builder
	for index := 0; index < len(source); {
		if language == "html" && source[index] == '<' {
			if end := syntaxTagEnd(source, index); end > index {
				writeSyntaxToken(&output, "tag", source[index:end])
				index = end
				continue
			}
		}
		if end, kind := syntaxCommentEnd(source, index, language); end > index {
			writeSyntaxToken(&output, kind, source[index:end])
			index = end
			continue
		}
		if quote := syntaxQuoteAt(source, index, language); quote != "" {
			end := syntaxStringEnd(source, index, quote)
			writeSyntaxToken(&output, "string", source[index:end])
			index = end
			continue
		}
		r, size := utf8.DecodeRuneInString(source[index:])
		if unicode.IsDigit(r) {
			end := index + size
			for end < len(source) {
				next, nextSize := utf8.DecodeRuneInString(source[end:])
				if !unicode.IsDigit(next) && next != '.' && next != '_' {
					break
				}
				end += nextSize
			}
			writeSyntaxToken(&output, "number", source[index:end])
			index = end
			continue
		}
		if syntaxIdentifierStart(r, language) {
			end := index + size
			for end < len(source) {
				next, nextSize := utf8.DecodeRuneInString(source[end:])
				if !syntaxIdentifierPart(next) {
					break
				}
				end += nextSize
			}
			token := source[index:end]
			if syntaxVariable(language, token) {
				writeSyntaxToken(&output, "variable", token)
			} else if (language == "css" || language == "scss" || language == "less") && strings.HasPrefix(token, "@") && isSyntaxKeyword(language, token[1:]) {
				writeSyntaxToken(&output, "keyword", token)
			} else if isSyntaxKeyword(language, token) {
				writeSyntaxToken(&output, "keyword", token)
			} else {
				output.WriteString(html.EscapeString(token))
			}
			index = end
			continue
		}
		output.WriteString(html.EscapeString(source[index : index+size]))
		index += size
	}
	return output.String()
}

func syntaxVariable(language, token string) bool {
	if strings.HasPrefix(token, "$") {
		return language == "javascript" || language == "typescript" || language == "php" || language == "scss"
	}
	return strings.HasPrefix(token, "@") && (language == "less" || language == "scss")
}

func isSyntaxKeyword(language, token string) bool {
	if _, ok := syntaxKeywords[language][token]; ok {
		return true
	}
	if language == "sql" || language == "php" || language == "css" || language == "scss" || language == "less" {
		_, ok := syntaxKeywords[language][strings.ToLower(token)]
		return ok
	}
	return false
}

func syntaxTagEnd(source string, start int) int {
	if start+1 >= len(source) {
		return start
	}
	switch source[start+1] {
	case '/', '!', '?':
	default:
		if !isASCIIAlpha(source[start+1]) {
			return start
		}
	}
	quote := byte(0)
	for index := start + 1; index < len(source); index++ {
		current := source[index]
		if quote != 0 {
			if current == quote {
				quote = 0
			}
			continue
		}
		switch current {
		case '"', '\'':
			quote = current
		case '>':
			return index + 1
		}
	}
	return start
}

func syntaxCommentEnd(source string, start int, language string) (int, string) {
	if strings.HasPrefix(source[start:], "<!--") {
		if end := strings.Index(source[start+4:], "-->"); end >= 0 {
			return start + 4 + end + 3, "comment"
		}
		return len(source), "comment"
	}
	if strings.HasPrefix(source[start:], "/*") {
		if end := strings.Index(source[start+2:], "*/"); end >= 0 {
			return start + 2 + end + 2, "comment"
		}
		return len(source), "comment"
	}
	lineComment := strings.HasPrefix(source[start:], "//") && (language == "javascript" || language == "typescript" || language == "php")
	lineComment = lineComment || strings.HasPrefix(source[start:], "--") && language == "sql"
	lineComment = lineComment || source[start] == '#' && language == "python"
	if lineComment {
		if end := strings.IndexByte(source[start:], '\n'); end >= 0 {
			return start + end, "comment"
		}
		return len(source), "comment"
	}
	return start, ""
}

func syntaxQuoteAt(source string, index int, language string) string {
	if index >= len(source) {
		return ""
	}
	quote := source[index]
	switch quote {
	case '"', '\'':
	case '`':
		if language != "javascript" && language != "typescript" {
			return ""
		}
	default:
		return ""
	}
	if quote == '\'' || quote == '"' {
		if language == "python" && strings.HasPrefix(source[index:], strings.Repeat(string(quote), 3)) {
			return strings.Repeat(string(quote), 3)
		}
	}
	return string(quote)
}

func syntaxStringEnd(source string, start int, delimiter string) int {
	for index := start + len(delimiter); index < len(source); index++ {
		if source[index] == '\\' {
			index++
			continue
		}
		if strings.HasPrefix(source[index:], delimiter) {
			return index + len(delimiter)
		}
	}
	return len(source)
}

func syntaxIdentifierStart(r rune, language string) bool {
	return unicode.IsLetter(r) || r == '_' || r == '$' && (language == "javascript" || language == "typescript" || language == "php" || language == "scss") || r == '@' && (language == "css" || language == "scss" || language == "less")
}

func syntaxIdentifierPart(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '$' || r == '-'
}

func isASCIIAlpha(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func writeSyntaxToken(output *strings.Builder, kind, value string) {
	output.WriteString(`<span class="syntax-` + kind + `">`)
	output.WriteString(html.EscapeString(value))
	output.WriteString("</span>")
}
