package swagger

import (
	"encoding/json"

	"github.com/apimgr/search/src/config"
)

// GenerateJSON returns the deterministic, indented JSON artifact embedded by
// the server and served through the OpenAPI routes.
func GenerateJSON(cfg *config.Config, baseURL string) ([]byte, error) {
	spec := GenerateSpec(cfg, baseURL)
	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
