package release

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	reposchema "github.com/Eriyc/wailsrel/schema"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const releaseManifestSchemaName = "release-manifest.schema.json"

type Manifest struct {
	SchemaVersion   int                      `json:"schema_version"`
	App             ManifestApp              `json:"app"`
	Release         ManifestRelease          `json:"release"`
	GeneratedAt     time.Time                `json:"generated_at"`
	Artifacts       []ManifestArtifact       `json:"artifacts"`
	Delta           *ManifestDelta           `json:"delta,omitempty"`
	Patches         []ManifestPatch          `json:"patches,omitempty"`
	FrontendBundles []ManifestFrontendBundle `json:"frontend_bundles,omitempty"`
}

type ManifestApp struct {
	Name       string `json:"name"`
	Identifier string `json:"identifier"`
}

type ManifestRelease struct {
	Tag      string `json:"tag"`
	Version  string `json:"version"`
	Provider string `json:"provider"`
}

type ManifestArtifact struct {
	Path      string            `json:"path"`
	AssetName string            `json:"asset_name"`
	OS        string            `json:"os"`
	Arch      string            `json:"arch"`
	Format    string            `json:"format"`
	Transport string            `json:"transport"`
	Checksum  string            `json:"checksum"`
	Size      int64             `json:"size"`
	URL       string            `json:"url"`
	Metadata  map[string]string `json:"metadata"`
}

type ManifestDelta struct {
	ManifestURL string `json:"manifest_url"`
}

type ManifestPatch struct {
	FromVersion  string `json:"from_version"`
	Artifact     string `json:"artifact"`
	URL          string `json:"url"`
	Checksum     string `json:"checksum"`
	FromChecksum string `json:"from_checksum"`
	ToChecksum   string `json:"to_checksum"`
	Size         int64  `json:"size"`
}

type ManifestFrontendBundle struct {
	Channel  string `json:"channel"`
	Version  string `json:"version"`
	CompatID string `json:"compat_id"`
	URL      string `json:"url"`
	Checksum string `json:"checksum"`
	Size     int64  `json:"size"`
}

func WriteManifest(manifest *Manifest, path string) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func ValidateManifest(manifest *Manifest) error {
	schemaData, err := reposchema.ReadFile(releaseManifestSchemaName)
	if err != nil {
		return err
	}

	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(releaseManifestSchemaName, anyJSON(schemaData)); err != nil {
		return err
	}
	schema, err := compiler.Compile(releaseManifestSchemaName)
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
