package contract_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Eriyc/wailsrel/pkg/delta"
	"github.com/Eriyc/wailsrel/pkg/frontend"
	"github.com/Eriyc/wailsrel/pkg/release"
)

func TestReleaseManifestJSONAndProtobufRoundTrip(t *testing.T) {
	manifest := &release.Manifest{
		SchemaVersion: 1,
		App: release.ManifestApp{
			Name:       "MyApp",
			Identifier: "com.example.myapp",
		},
		Release: release.ManifestRelease{
			Tag:      "v1.2.3",
			Version:  "1.2.3",
			Provider: release.ProviderHTTP,
		},
		GeneratedAt: time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC),
		Artifacts: []release.ManifestArtifact{
			{
				Path:      "linux/amd64/MyApp.AppImage",
				AssetName: "MyApp-1.2.3-linux-amd64-appimage.AppImage",
				OS:        "linux",
				Arch:      "amd64",
				Format:    "appimage",
				Transport: "file",
				Checksum:  "sha256:" + strings.Repeat("a", 64),
				Size:      123,
				URL:       "https://releases.example.com/download/v1.2.3/MyApp.AppImage",
				Metadata: map[string]string{
					"compat_id": "compat-123",
				},
			},
		},
		Delta: &release.ManifestDelta{ManifestURL: "https://releases.example.com/delta/manifest"},
		FrontendBundles: []release.ManifestFrontendBundle{
			{
				Channel:  "stable",
				Version:  "1.2.3",
				CompatID: "compat-123",
				URL:      "https://releases.example.com/download/v1.2.3/frontend-stable-1.2.3.zip",
				Checksum: "sha256:" + strings.Repeat("b", 64),
				Size:     456,
			},
		},
	}

	jsonData, err := release.EncodeManifestJSON(manifest)
	if err != nil {
		t.Fatalf("encode manifest json: %v", err)
	}
	jsonDecoded, err := release.DecodeManifest(jsonData, "application/json")
	if err != nil {
		t.Fatalf("decode manifest json: %v", err)
	}

	protoData, err := release.EncodeManifestProtobuf(manifest)
	if err != nil {
		t.Fatalf("encode manifest protobuf: %v", err)
	}
	protoDecoded, err := release.DecodeManifest(protoData, "application/x-protobuf")
	if err != nil {
		t.Fatalf("decode manifest protobuf: %v", err)
	}

	if !reflect.DeepEqual(jsonDecoded, protoDecoded) {
		t.Fatalf("manifest round-trip mismatch\njson=%+v\nproto=%+v", jsonDecoded, protoDecoded)
	}
}

func TestDeltaManifestJSONAndProtobufRoundTrip(t *testing.T) {
	manifest := &delta.PatchManifest{
		SchemaVersion: 1,
		Algorithm:     "bsdiff",
		GeneratedAt:   time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC),
		Patches: []delta.PatchManifestEntry{
			{
				FromVersion:    "1.0.0",
				Artifact:       "linux/amd64/MyApp.AppImage",
				Patch:          "/download/v1.2.3/MyApp.patch",
				ArtifactKind:   delta.ArtifactFile,
				FromSHA256:     strings.Repeat("a", 64),
				ToSHA256:       strings.Repeat("b", 64),
				PatchSHA256:    strings.Repeat("c", 64),
				FromSize:       100,
				ToSize:         120,
				PatchSize:      20,
				SavingsBytes:   80,
				SavingsPercent: 80,
			},
		},
	}

	jsonData, err := delta.EncodeManifestJSON(manifest)
	if err != nil {
		t.Fatalf("encode delta json: %v", err)
	}
	jsonDecoded, err := delta.DecodeManifest(jsonData, "application/json")
	if err != nil {
		t.Fatalf("decode delta json: %v", err)
	}

	protoData, err := delta.EncodeManifestProtobuf(manifest)
	if err != nil {
		t.Fatalf("encode delta protobuf: %v", err)
	}
	protoDecoded, err := delta.DecodeManifest(protoData, "application/x-protobuf")
	if err != nil {
		t.Fatalf("decode delta protobuf: %v", err)
	}

	if !reflect.DeepEqual(jsonDecoded, protoDecoded) {
		t.Fatalf("delta round-trip mismatch\njson=%+v\nproto=%+v", jsonDecoded, protoDecoded)
	}
}

func TestFrontendCatalogJSONAndProtobufRoundTrip(t *testing.T) {
	catalog := frontend.Catalog{
		SchemaVersion: frontend.CatalogSchemaVersion,
		AppID:         "com.example.myapp",
		GeneratedAt:   time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC),
		Codepush: []frontend.CodepushEntry{
			{
				Name:        "hotfix-1",
				Version:     "1.2.4",
				CompatID:    "compat-123",
				URL:         "https://releases.example.com/download/v1.2.4/frontend-stable-1.2.4.zip",
				Checksum:    "sha256:" + strings.Repeat("d", 64),
				Size:        100,
				Force:       true,
				PublishedAt: time.Date(2026, 3, 13, 12, 30, 0, 0, time.UTC),
			},
		},
		Experiments: []frontend.ExperimentEntry{
			{
				Name:        "green",
				Version:     "1.2.4",
				CompatID:    "compat-123",
				URL:         "https://releases.example.com/download/v1.2.4/frontend-green-1.2.4.zip",
				Checksum:    "sha256:" + strings.Repeat("e", 64),
				Size:        200,
				DisplayName: "Green",
				Description: "Green experiment",
				PublishedAt: time.Date(2026, 3, 13, 12, 45, 0, 0, time.UTC),
			},
		},
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	payload, err := frontend.CanonicalCatalogPayload(catalog)
	if err != nil {
		t.Fatalf("canonical catalog payload: %v", err)
	}
	catalog.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))

	jsonData, err := frontend.EncodeCatalogJSON(catalog)
	if err != nil {
		t.Fatalf("encode catalog json: %v", err)
	}
	jsonDecoded, err := frontend.DecodeCatalogResponse(jsonData, "application/json", base64.StdEncoding.EncodeToString(publicKey), catalog.AppID)
	if err != nil {
		t.Fatalf("decode catalog json: %v", err)
	}

	protoData, err := frontend.EncodeCatalogProtobuf(catalog)
	if err != nil {
		t.Fatalf("encode catalog protobuf: %v", err)
	}
	protoDecoded, err := frontend.DecodeCatalogResponse(protoData, "application/x-protobuf", base64.StdEncoding.EncodeToString(publicKey), catalog.AppID)
	if err != nil {
		t.Fatalf("decode catalog protobuf: %v", err)
	}

	if len(jsonData) == 0 || len(protoData) == 0 {
		t.Fatal("expected contract encoders to produce non-empty outputs")
	}
	if !reflect.DeepEqual(jsonDecoded, protoDecoded) {
		t.Fatalf("catalog round-trip mismatch\njson=%+v\nproto=%+v", jsonDecoded, protoDecoded)
	}
}
