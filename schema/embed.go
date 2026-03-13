package schema

import (
	"embed"
	"fmt"
)

// FS contains the repository JSON schema files.
//
//go:embed *.schema.json generated/*.json
var FS embed.FS

func ReadFile(name string) ([]byte, error) {
	data, err := FS.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read schema %s: %w", name, err)
	}
	return data, nil
}
