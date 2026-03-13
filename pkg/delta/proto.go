package delta

import (
	"encoding/json"
	"path"

	"github.com/Eriyc/wailsrel/gen/go/wailsrel/v1"
	"github.com/Eriyc/wailsrel/pkg/contract"
)

const ManifestAssetProtoName = "delta-manifest.pb"

func EncodeManifestJSON(manifest *PatchManifest) ([]byte, error) {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func EncodeManifestProtobuf(manifest *PatchManifest) ([]byte, error) {
	return contract.MarshalProtobuf(manifestToProto(manifest))
}

func DecodeManifest(data []byte, contentType string) (*PatchManifest, error) {
	message := &wailsrelv1.PublicDeltaManifest{}
	if err := contract.Unmarshal(data, contentType, message); err != nil {
		return nil, err
	}
	return manifestFromProto(message), nil
}

func EncodeManifestFile(manifest *PatchManifest, outputPath string) ([]byte, error) {
	switch path.Ext(outputPath) {
	case ".pb":
		return EncodeManifestProtobuf(manifest)
	default:
		return EncodeManifestJSON(manifest)
	}
}

func manifestToProto(manifest *PatchManifest) *wailsrelv1.PublicDeltaManifest {
	if manifest == nil {
		return &wailsrelv1.PublicDeltaManifest{}
	}

	out := &wailsrelv1.PublicDeltaManifest{
		SchemaVersion: int32(manifest.SchemaVersion),
		Algorithm:     manifest.Algorithm,
		GeneratedAt:   contract.Timestamp(manifest.GeneratedAt),
		Patches:       make([]*wailsrelv1.PublicDeltaManifestEntry, 0, len(manifest.Patches)),
	}
	for _, patch := range manifest.Patches {
		out.Patches = append(out.Patches, &wailsrelv1.PublicDeltaManifestEntry{
			FromVersion:    patch.FromVersion,
			Artifact:       patch.Artifact,
			Patch:          patch.Patch,
			ArtifactKind:   string(patch.ArtifactKind),
			FromSha256:     patch.FromSHA256,
			ToSha256:       patch.ToSHA256,
			PatchSha256:    patch.PatchSHA256,
			FromSize:       patch.FromSize,
			ToSize:         patch.ToSize,
			PatchSize:      patch.PatchSize,
			SavingsBytes:   patch.SavingsBytes,
			SavingsPercent: patch.SavingsPercent,
		})
	}
	return out
}

func manifestFromProto(message *wailsrelv1.PublicDeltaManifest) *PatchManifest {
	if message == nil {
		return &PatchManifest{}
	}

	out := &PatchManifest{
		SchemaVersion: int(message.SchemaVersion),
		Algorithm:     message.Algorithm,
		GeneratedAt:   contract.TimeValue(message.GeneratedAt),
		Patches:       make([]PatchManifestEntry, 0, len(message.Patches)),
	}
	for _, patch := range message.Patches {
		if patch == nil {
			continue
		}
		out.Patches = append(out.Patches, PatchManifestEntry{
			FromVersion:    patch.FromVersion,
			Artifact:       patch.Artifact,
			Patch:          firstNonEmpty(patch.Patch, patch.AssetKey),
			ArtifactKind:   ArtifactKind(patch.ArtifactKind),
			FromSHA256:     patch.FromSha256,
			ToSHA256:       patch.ToSha256,
			PatchSHA256:    patch.PatchSha256,
			FromSize:       patch.FromSize,
			ToSize:         patch.ToSize,
			PatchSize:      patch.PatchSize,
			SavingsBytes:   patch.SavingsBytes,
			SavingsPercent: patch.SavingsPercent,
		})
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
