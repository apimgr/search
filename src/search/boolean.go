package search

import (
	"fmt"
	"strings"
	"unicode"
)

type booleanTokenType uint8

const (
	booleanTerm booleanTokenType = iota
	booleanAnd
	booleanOr
	booleanNot
	booleanLeftParen
	booleanRightParen
)

type booleanToken struct {
	typeOf booleanTokenType
	value  string
}

type booleanNode struct {
	operator booleanTokenType
	value    string
	children []*booleanNode
}

func normalizeBooleanQuery(query string) (string, error) {
	tokens, err := lexBooleanQuery(query)
	if err != nil || len(tokens) == 0 {
		return "", err
	}
	parser := booleanParser{tokens: tokens}
	root, err := parser.parseOr()
	if err != nil {
		return "", err
	}
	if parser.position != len(parser.tokens) {
		return "", fmt.Errorf("unexpected token %q in Boolean search query", parser.tokens[parser.position].value)
	}
	return renderBooleanNode(root, false, 0), nil
}

func lexBooleanQuery(query string) ([]booleanToken, error) {
	tokens := make([]booleanToken, 0, len(query)/4)
	for index := 0; index < len(query); {
		if unicode.IsSpace(rune(query[index])) {
			index++
			continue
		}
		switch query[index] {
		case '(':
			tokens = append(tokens, booleanToken{typeOf: booleanLeftParen, value: "("})
			index++
			continue
		case ')':
			tokens = append(tokens, booleanToken{typeOf: booleanRightParen, value: ")"})
			index++
			continue
		case '"':
			start := index
			index++
			escaped, closed := false, false
			for index < len(query) {
				if query[index] == '"' && !escaped {
					index++
					closed = true
					break
				}
				if query[index] == '\\' && !escaped {
					escaped = true
				} else {
					escaped = false
				}
				index++
			}
			if !closed {
				return nil, fmt.Errorf("unclosed quoted phrase in Boolean search query")
			}
			tokens = append(tokens, booleanToken{typeOf: booleanTerm, value: query[start:index]})
			continue
		}
		start := index
		for index < len(query) && !unicode.IsSpace(rune(query[index])) && query[index] != '(' && query[index] != ')' {
			index++
		}
		value := query[start:index]
		token := booleanToken{typeOf: booleanTerm, value: value}
		switch {
		case strings.EqualFold(value, "AND"):
			token.typeOf = booleanAnd
		case strings.EqualFold(value, "OR"):
			token.typeOf = booleanOr
		case strings.EqualFold(value, "NOT"):
			token.typeOf = booleanNot
		}
		tokens = append(tokens, token)
	}
	return tokens, nil
}

type booleanParser struct {
	tokens   []booleanToken
	position int
}

func (p *booleanParser) parseOr() (*booleanNode, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	children := []*booleanNode{left}
	for p.peek(booleanOr) {
		p.position++
		right, parseErr := p.parseAnd()
		if parseErr != nil {
			return nil, parseErr
		}
		children = append(children, right)
	}
	if len(children) == 1 {
		return left, nil
	}
	return &booleanNode{operator: booleanOr, children: children}, nil
}

func (p *booleanParser) parseAnd() (*booleanNode, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	children := []*booleanNode{left}
	for p.position < len(p.tokens) {
		if p.peek(booleanAnd) {
			p.position++
		} else if !p.peek(booleanTerm) && !p.peek(booleanNot) && !p.peek(booleanLeftParen) {
			break
		}
		right, parseErr := p.parseUnary()
		if parseErr != nil {
			return nil, parseErr
		}
		children = append(children, right)
	}
	if len(children) == 1 {
		return left, nil
	}
	return &booleanNode{operator: booleanAnd, children: children}, nil
}

func (p *booleanParser) parseUnary() (*booleanNode, error) {
	if p.peek(booleanNot) {
		p.position++
		child, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &booleanNode{operator: booleanNot, children: []*booleanNode{child}}, nil
	}
	if p.peek(booleanLeftParen) {
		p.position++
		child, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if !p.peek(booleanRightParen) {
			return nil, fmt.Errorf("unclosed group in Boolean search query")
		}
		p.position++
		return child, nil
	}
	if p.position >= len(p.tokens) || !p.peek(booleanTerm) {
		return nil, fmt.Errorf("expected a search term in Boolean search query")
	}
	token := p.tokens[p.position]
	p.position++
	return &booleanNode{operator: booleanTerm, value: token.value}, nil
}

func (p *booleanParser) peek(tokenType booleanTokenType) bool {
	return p.position < len(p.tokens) && p.tokens[p.position].typeOf == tokenType
}

func renderBooleanNode(node *booleanNode, negate bool, parentPrecedence int) string {
	if node.operator == booleanNot {
		return renderBooleanNode(node.children[0], !negate, parentPrecedence)
	}
	if node.operator == booleanTerm {
		if negate {
			return "-" + node.value
		}
		return node.value
	}
	operator := node.operator
	if negate {
		if operator == booleanAnd {
			operator = booleanOr
		} else {
			operator = booleanAnd
		}
	}
	precedence, separator := 2, " "
	if operator == booleanOr {
		precedence, separator = 1, " OR "
	}
	children := make([]string, 0, len(node.children))
	for _, child := range node.children {
		children = append(children, renderBooleanNode(child, negate, precedence))
	}
	result := strings.Join(children, separator)
	if precedence < parentPrecedence {
		return "(" + result + ")"
	}
	return result
}
