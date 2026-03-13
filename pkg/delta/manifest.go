package delta

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	reposchema "github.com/you/wailsrel/schema"
)

const manifestSchemaName = "delta-manifest.schema.json"

type PatchManifest struct {
	SchemaVersion int                  `json:"schema_version"`
	Algorithm     string               `json:"algorithm"`
	GeneratedAt   time.Time            `json:"generated_at"`
	Patches       []PatchManifestEntry `json:"patches"`
}

type PatchManifestEntry struct {
	FromVersion    string       `json:"from_version"`
	Artifact       string       `json:"artifact"`
	Patch          string       `json:"patch"`
	ArtifactKind   ArtifactKind `json:"artifact_kind"`
	FromSHA256     string       `json:"from_sha256"`
	ToSHA256       string       `json:"to_sha256"`
	PatchSHA256    string       `json:"patch_sha256"`
	FromSize       int64        `json:"from_size"`
	ToSize         int64        `json:"to_size"`
	PatchSize      int64        `json:"patch_size"`
	SavingsBytes   int64        `json:"savings_bytes,omitempty"`
	SavingsPercent float64      `json:"savings_percent,omitempty"`
}

func BuildManifest(outputDir string, generated []Generated) *PatchManifest {
	manifest := &PatchManifest{
		SchemaVersion: 1,
		Algorithm:     "bsdiff",
		GeneratedAt:   time.Now().UTC(),
		Patches:       make([]PatchManifestEntry, 0, len(generated)),
	}

	for _, item := range generated {
		patchPath := filepath.ToSlash(item.Patch)
		if outputDir != "" {
			if rel, err := filepath.Rel(outputDir, item.Patch); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
				patchPath = filepath.ToSlash(rel)
			}
		}
		manifest.Patches = append(manifest.Patches, PatchManifestEntry{
			FromVersion:    item.FromVersion,
			Artifact:       item.Artifact,
			Patch:          patchPath,
			ArtifactKind:   item.ArtifactKind,
			FromSHA256:     strings.TrimPrefix(item.FromChecksum, "sha256:"),
			ToSHA256:       strings.TrimPrefix(item.ToChecksum, "sha256:"),
			PatchSHA256:    strings.TrimPrefix(item.Checksum, "sha256:"),
			FromSize:       item.FromSize,
			ToSize:         item.ToSize,
			PatchSize:      item.Size,
			SavingsBytes:   item.SavingsBytes,
			SavingsPercent: item.SavingsPercent,
		})
	}

	return manifest
}

func WriteManifest(manifest *PatchManifest, path string) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	return nil
}

func ValidateManifest(manifest *PatchManifest) error {
	schemaData, err := reposchema.ReadFile(manifestSchemaName)
	if err != nil {
		return err
	}

	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(manifestSchemaName, anyJSON(schemaData)); err != nil {
		return err
	}
	schema, err := compiler.Compile(manifestSchemaName)
	if err != nil {
		return err
	}
	instance, err := anyJSONValue(manifest)
	if err != nil {
		return err
	}
	return schema.Validate(instance)
}

func anyJSON(data []byte) any {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		panic(fmt.Sprintf("invalid embedded JSON schema: %v", err))
	}
	return value
}

func anyJSONValue(value any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}
