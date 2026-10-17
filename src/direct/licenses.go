package direct

import (
	"embed"
	"io/fs"
	"strings"
)

// SPDX license texts are embedded from the SPDX License List text data.
//
//go:embed license_text/*.txt
var licenseTexts embed.FS

func fullLicenseText(spdx string) string {
	id := spdx
	if strings.HasPrefix(id, "GPL-") || strings.HasPrefix(id, "AGPL-") || strings.HasPrefix(id, "LGPL-") {
		id += "-only"
	}
	text, err := fs.ReadFile(licenseTexts, "license_text/"+id+".txt")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(text))
}
