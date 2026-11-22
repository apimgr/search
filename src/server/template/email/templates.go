package emailtemplates

import "embed"

// Defaults contains the PART 17 email templates embedded in the server binary.
//
//go:embed *.txt
var Defaults embed.FS
