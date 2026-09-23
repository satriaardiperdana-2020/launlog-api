package api

import "embed"

// OpenAPISpec is embedded so the development server can serve the same
// contract that code generation and documentation use.
//
//go:embed openapi.yaml
var OpenAPISpec embed.FS
