package indexes

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Eriyc/wailsrel/gen/go/wailsrel/v1"
	"github.com/Eriyc/wailsrel/pkg/build"
	"github.com/Eriyc/wailsrel/pkg/config"
	"github.com/Eriyc/wailsrel/pkg/contract"
	"github.com/Eriyc/wailsrel/pkg/frontend"
	"github.com/Eriyc/wailsrel/pkg/release"
	"google.golang.org/protobuf/proto"
)

const (
	ReleaseIndexJSONName   = "release-index.json"
	ReleaseIndexProtoName  = "release-index.pb"
	FrontendIndexJSONName  = "frontend-index.json"
	FrontendIndexProtoName = "frontend-index.pb"
)

type ReleaseOptions struct {
	App            config.AppConfig
	Tag            string
	Version        string
	PublishedAt    time.Time
	NativeCompatID string
	Artifacts      []release.PublishedAsset
	Frontend       []release.PublishedFrontendBundle
}

type FrontendOptions struct {
	AppID       string
	Tag         string
	PublishedAt time.Time
	Bundles     []frontend.BundleArtifact
}

func BuildReleaseIndex(opts ReleaseOptions) *wailsrelv1.ReleaseIndex {
	entry := &wailsrelv1.ReleaseEntry{
		Tag:             strings.TrimSpace(opts.Tag),
		Version:         strings.TrimSpace(opts.Version),
		CompatId:        strings.TrimSpace(opts.NativeCompatID),
		PublishedAt:     contract.Timestamp(opts.PublishedAt),
		Assets:          make([]*wailsrelv1.PlatformAsset, 0, len(opts.Artifacts)),
		FrontendBundles: make([]*wailsrelv1.BundleEntry, 0, len(opts.Frontend)),
	}
	for _, artifact := range opts.Artifacts {
		metadata := cloneMetadata(artifact.Metadata)
		entry.Assets = append(entry.Assets, &wailsrelv1.PlatformAsset{
			LogicalPath:  artifact.LogicalPath,
			AssetKey:     artifact.AssetName,
			Os:           artifact.OS,
			Arch:         artifact.Arch,
			Format:       artifact.Format,
			Transport:    artifact.Transport,
			Checksum:     artifact.Checksum,
			Size:         artifact.Size,
			CompatId:     firstNonEmpty(metadata["compat_id"], metadata["native_compat"], opts.NativeCompatID),
			Channel:      metadata["channel"],
			Force:        metadata["force"] == "true",
			PublishedAt:  contract.Timestamp(opts.PublishedAt),
			SourceBranch: metadata["source_branch"],
			CommitSha:    metadata["commit_sha"],
			Metadata:     metadata,
		})
	}
	for _, bundle := range opts.Frontend {
		entry.FrontendBundles = append(entry.FrontendBundles, &wailsrelv1.BundleEntry{
			Tag:          opts.Tag,
			Kind:         firstNonEmpty(bundle.Kind, frontend.BundleKindCodepush),
			Name:         firstNonEmpty(bundle.Name, bundle.Channel),
			Version:      bundle.Version,
			CompatId:     bundle.CompatID,
			AssetKey:     bundle.AssetName,
			Checksum:     bundle.Checksum,
			Size:         bundle.Size,
			Force:        bundle.Force,
			Channel:      bundle.Channel,
			PublishedAt:  contract.Timestamp(opts.PublishedAt),
			SourceBranch: bundle.SourceBranch,
			CommitSha:    bundle.CommitSHA,
		})
	}
	return &wailsrelv1.ReleaseIndex{
		SchemaVersion: 1,
		AppName:       opts.App.Name,
		AppIdentifier: opts.App.Identifier,
		GeneratedAt:   contract.Timestamp(opts.PublishedAt),
		Releases:      []*wailsrelv1.ReleaseEntry{entry},
	}
}

func BuildFrontendIndex(opts FrontendOptions) *wailsrelv1.FrontendIndex {
	index := &wailsrelv1.FrontendIndex{
		SchemaVersion: 1,
		AppId:         strings.TrimSpace(opts.AppID),
		GeneratedAt:   contract.Timestamp(opts.PublishedAt),
		Bundles:       make([]*wailsrelv1.BundleEntry, 0, len(opts.Bundles)),
	}
	for _, bundle := range opts.Bundles {
		checksum, size := bundleChecksum(bundle)
		index.Bundles = append(index.Bundles, &wailsrelv1.BundleEntry{
			Tag:          opts.Tag,
			Kind:         strings.TrimSpace(bundle.Manifest.Kind),
			Name:         firstNonEmpty(strings.TrimSpace(bundle.Manifest.Name), strings.TrimSpace(bundle.Manifest.Channel)),
			Version:      firstNonEmpty(strings.TrimSpace(bundle.Manifest.Version), strings.TrimSpace(bundle.Manifest.BundleVersion)),
			CompatId:     strings.TrimSpace(bundle.Manifest.CompatID),
			AssetKey:     filepath.Base(bundle.Path),
			Checksum:     checksum,
			Size:         size,
			Force:        bundle.Manifest.Force,
			Channel:      strings.TrimSpace(bundle.Manifest.Channel),
			PublishedAt:  contract.Timestamp(opts.PublishedAt),
			SourceBranch: strings.TrimSpace(bundle.Manifest.SourceBranch),
			CommitSha:    strings.TrimSpace(bundle.Manifest.CommitSHA),
		})
	}
	return index
}

func WriteReleaseIndexFiles(dir string, index *wailsrelv1.ReleaseIndex) (string, string, error) {
	jsonPath := filepath.Join(dir, ReleaseIndexJSONName)
	protoPath := filepath.Join(dir, ReleaseIndexProtoName)
	if err := writeMessage(jsonPath, index, contract.ContentTypeJSON); err != nil {
		return "", "", err
	}
	if err := writeMessage(protoPath, index, contract.ContentTypeProtobuf); err != nil {
		return "", "", err
	}
	return jsonPath, protoPath, nil
}

func WriteFrontendIndexFiles(dir string, index *wailsrelv1.FrontendIndex) (string, string, error) {
	jsonPath := filepath.Join(dir, FrontendIndexJSONName)
	protoPath := filepath.Join(dir, FrontendIndexProtoName)
	if err := writeMessage(jsonPath, index, contract.ContentTypeJSON); err != nil {
		return "", "", err
	}
	if err := writeMessage(protoPath, index, contract.ContentTypeProtobuf); err != nil {
		return "", "", err
	}
	return jsonPath, protoPath, nil
}

func writeMessage(path string, message proto.Message, contentType string) error {
	data, err := contract.Marshal(message, contentType)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func bundleChecksum(bundle frontend.BundleArtifact) (string, int64) {
	if strings.TrimSpace(bundle.Manifest.Checksum) != "" && bundle.Size > 0 {
		return bundle.Manifest.Checksum, bundle.Size
	}
	info, err := os.Stat(bundle.Path)
	if err == nil && info != nil {
		if checksum, checksumErr := build.ComputeChecksum(bundle.Path); checksumErr == nil {
			return "sha256:" + checksum, info.Size()
		}
		return "", info.Size()
	}
	return "", bundle.Size
}

func cloneMetadata(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
