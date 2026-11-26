package main

import (
	"flag"
	"log"
	"os"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/swagger"
)

func main() {
	output := flag.String("output", "src/swagger/openapi.json", "path for the generated OpenAPI JSON artifact")
	flag.Parse()

	data, err := swagger.GenerateJSON(config.DefaultConfig(), "https://generated.invalid")
	if err != nil {
		log.Fatalf("generate OpenAPI spec: %v", err)
	}
	if err := os.WriteFile(*output, data, 0o644); err != nil {
		log.Fatalf("write OpenAPI spec %q: %v", *output, err)
	}
}
