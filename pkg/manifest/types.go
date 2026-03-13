package manifest

import "time"

type ReleaseManifest struct {
	SchemaVersion   int             `json:"schema_version"`
	AppName         string          `json:"app_name"`
	Version         string          `json:"version"`
	Channel         string          `json:"channel"`
	Date            time.Time       `json:"date"`
	ReleaseNotes    string          `json:"release_notes,omitempty"`
	NativeCompat    string          `json:"native_compat_id"`
	Artifacts       []ArtifactEntry `json:"artifacts"`
	Patches         []PatchEntry    `json:"patches,omitempty"`
	FrontendBundles []FrontendEntry `json:"frontend_bundles,omitempty"`
	Mandatory       *MandatoryInfo  `json:"mandatory,omitempty"`
}

type ArtifactEntry struct {
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Format   string `json:"format"`
	URL      string `json:"url"`
	Checksum string `json:"checksum"`
	Size     int64  `json:"size"`
}

type PatchEntry struct {
	FromVersion string `json:"from_version"`
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	FromHash    string `json:"from_hash"`
	ToHash      string `json:"to_hash"`
	URL         string `json:"url"`
	Checksum    string `json:"checksum"`
	Size        int64  `json:"size"`
}

type FrontendEntry struct {
	Channel  string `json:"channel"`
	Version  string `json:"version"`
	CompatID string `json:"compat_id"`
	URL      string `json:"url"`
	Checksum string `json:"checksum"`
	Size     int64  `json:"size"`
}

type MandatoryInfo struct {
	MinVersion string `json:"min_version"`
	Message    string `json:"message"`
}

type GenerateOpts struct {
	AppName             string
	Version             string
	Channel             string
	Date                time.Time
	ReleaseNotes        string
	NativeCompat        string
	Mandatory           *MandatoryInfo
	ArtifactURLTemplate string
	PatchURLTemplate    string
	FrontendURLTemplate string
	Artifacts           []GeneratedArtifact
	Patches             []GeneratedPatch
	FrontendBundles     []GeneratedFrontendBundle
}

type GeneratedArtifact struct {
	Name     string
	OS       string
	Arch     string
	Format   string
	URL      string
	Checksum string
	Size     int64
}

type GeneratedPatch struct {
	Name        string
	FromVersion string
	OS          string
	Arch        string
	FromHash    string
	ToHash      string
	URL         string
	Checksum    string
	Size        int64
}

type GeneratedFrontendBundle struct {
	Name     string
	Channel  string
	Version  string
	CompatID string
	URL      string
	Checksum string
	Size     int64
}
