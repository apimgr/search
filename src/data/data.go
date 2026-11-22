package data

import _ "embed"

// DNSProviderCatalog contains embedded DNS challenge provider metadata.
//
//go:embed dns_providers.json
var DNSProviderCatalog []byte
