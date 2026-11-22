package main

import (
	"testing"

	appgraphql "github.com/apimgr/search/src/graphql"
	"github.com/graphql-go/graphql/language/parser"
)

func TestGraphQLSDLIsValidAndDeterministic(t *testing.T) {
	if err := appgraphql.InitSchema(); err != nil {
		t.Fatalf("initialize GraphQL schema: %v", err)
	}

	first, err := graphQLSDL(&appgraphql.Schema)
	if err != nil {
		t.Fatalf("render GraphQL SDL: %v", err)
	}
	second, err := graphQLSDL(&appgraphql.Schema)
	if err != nil {
		t.Fatalf("render GraphQL SDL a second time: %v", err)
	}
	if first != second {
		t.Fatal("GraphQL SDL generation is not deterministic")
	}
	if _, err := parser.Parse(parser.ParseParams{Source: first}); err != nil {
		t.Fatalf("generated GraphQL SDL is invalid: %v", err)
	}
}
