package release

import (
	"encoding/json"

	"github.com/Eriyc/wailsrel/gen/go/wailsrel/v1"
	"github.com/Eriyc/wailsrel/pkg/contract"
)

func EncodeManifestJSON(manifest *Manifest) ([]byte, error) {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func EncodeManifestProtobuf(manifest *Manifest) ([]byte, error) {
	return contract.MarshalProtobuf(manifestToProto(manifest))
}

func decodeManifestProto(data []byte, contentType string) (*wailsrelv1.PublicReleaseManifest, error) {
	message := &wailsrelv1.PublicReleaseManifest{}
	if err := contract.Unmarshal(data, contentType, message); err != nil {
		return nil, err
	}
	return message, nil
}

func manifestToProto(manifest *Manifest) *wailsrelv1.PublicReleaseManifest {
	if manifest == nil {
		return &wailsrelv1.PublicReleaseManifest{}
	}

	out := &wailsrelv1.PublicReleaseManifest{
		SchemaVersion: int32(manifest.SchemaVersion),
		App: &wailsrelv1.ManifestApp{
			Name:       manifest.App.Name,
			Identifier: manifest.App.Identifier,
		},
		Release: &wailsrelv1.ManifestRelease{
			Tag:      manifest.Release.Tag,
			Version:  manifest.Release.Version,
			Provider: manifest.Release.Provider,
		},
		GeneratedAt:     contract.Timestamp(manifest.GeneratedAt),
		Artifacts:       make([]*wailsrelv1.ManifestArtifact, 0, len(manifest.Artifacts)),
		FrontendBundles: make([]*wailsrelv1.BundleEntry, 0, len(manifest.FrontendBundles)),
	}
	if manifest.Delta != nil {
		out.Delta = &wailsrelv1.ManifestDelta{ManifestUrl: manifest.Delta.ManifestURL}
	}
	for _, artifact := range manifest.Artifacts {
		out.Artifacts = append(out.Artifacts, &wailsrelv1.ManifestArtifact{
			Path:      artifact.Path,
			AssetName: artifact.AssetName,
			Os:        artifact.OS,
			Arch:      artifact.Arch,
			Format:    artifact.Format,
			Transport: artifact.Transport,
			Checksum:  artifact.Checksum,
			Size:      artifact.Size,
			Url:       artifact.URL,
			Metadata:  cloneMetadata(artifact.Metadata),
		})
	}
	for _, bundle := range manifest.FrontendBundles {
		out.FrontendBundles = append(out.FrontendBundles, &wailsrelv1.BundleEntry{
			Channel:  bundle.Channel,
			Version:  bundle.Version,
			CompatId: bundle.CompatID,
			Url:      bundle.URL,
			Checksum: bundle.Checksum,
			Size:     bundle.Size,
		})
	}
	return out
}

func manifestFromProto(message *wailsrelv1.PublicReleaseManifest) *Manifest {
	if message == nil {
		return &Manifest{}
	}

	out := &Manifest{
		SchemaVersion: int(message.SchemaVersion),
		App: ManifestApp{
			Name:       "",
			Identifier: "",
		},
		Release: ManifestRelease{
			Tag:      "",
			Version:  "",
			Provider: "",
		},
		GeneratedAt:     contract.TimeValue(message.GeneratedAt),
		Artifacts:       make([]ManifestArtifact, 0, len(message.Artifacts)),
		FrontendBundles: make([]ManifestFrontendBundle, 0, len(message.FrontendBundles)),
	}
	if message.App != nil {
		out.App.Name = message.App.Name
		out.App.Identifier = message.App.Identifier
	}
	if message.Release != nil {
		out.Release.Tag = message.Release.Tag
		out.Release.Version = message.Release.Version
		out.Release.Provider = message.Release.Provider
	}
	if message.Delta != nil {
		out.Delta = &ManifestDelta{ManifestURL: message.Delta.ManifestUrl}
	}
	for _, artifact := range message.Artifacts {
		if artifact == nil {
			continue
		}
		out.Artifacts = append(out.Artifacts, ManifestArtifact{
			Path:      artifact.Path,
			AssetName: artifact.AssetName,
			OS:        artifact.Os,
			Arch:      artifact.Arch,
			Format:    artifact.Format,
			Transport: artifact.Transport,
			Checksum:  artifact.Checksum,
			Size:      artifact.Size,
			URL:       artifact.Url,
			Metadata:  cloneMetadata(artifact.Metadata),
		})
	}
	for _, bundle := range message.FrontendBundles {
		if bundle == nil {
			continue
		}
		out.FrontendBundles = append(out.FrontendBundles, ManifestFrontendBundle{
			Channel:  bundle.Channel,
			Version:  bundle.Version,
			CompatID: bundle.CompatId,
			URL:      bundle.Url,
			Checksum: bundle.Checksum,
			Size:     bundle.Size,
		})
	}
	return out
}
