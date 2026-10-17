package engine

import (
	"net/url"
	"path"
	"strings"
)

func imageFormatFromURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	format := strings.TrimPrefix(strings.ToLower(path.Ext(parsed.Path)), ".")
	if format == "jpeg" {
		return "jpg"
	}
	switch format {
	case "jpg", "png", "gif", "webp":
		return format
	default:
		return ""
	}
}
