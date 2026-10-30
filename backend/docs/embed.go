package docs

import "embed"

// FS embeds the API contracts. Only openapi.yaml is served outside production.
// public-api.yaml remains a generation input for frontend docs and SDKs.
//
//go:embed openapi.yaml public-api.yaml
var FS embed.FS
