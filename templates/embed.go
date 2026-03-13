package templates

import "embed"

// FS contains embedded project templates used by the CLI.
//
//go:embed *.tmpl
var FS embed.FS
