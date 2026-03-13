package release

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/you/wailsrel/pkg/build"
	"github.com/you/wailsrel/pkg/config"
	"github.com/you/wailsrel/pkg/delta"
)

const (
	ManifestAssetName      = "manifest.json"
	DeltaManifestAssetName = "delta-manifest.json"
)

type UploadAsset struct {
	Name        string
	Path        string
	ContentType string
}

type PublishedAsset struct {
	LogicalPath string
	AssetName   string
	SourcePath  string
	OS          string
	Arch        string
	Format      string
	Transport   string
	Checksum    string
	Size        int64
	Metadata    map[string]string
}

type Bundle struct {
	Manifest          *Manifest
	ManifestPath      string
	DeltaManifestPath string
	Artifacts         []PublishedAsset
	Uploads           []UploadAsset
}

type BundleOptions struct {
	App       config.AppConfig
	OutputDir string
	TempDir   string
	Tag       string
	Version   string
	Resolver  Resolver
	Artifacts []build.Artifact
	Delta     *delta.Result
}

func PrepareBundle(opts BundleOptions) (*Bundle, error) {
	if opts.Resolver == nil {
		return nil, fmt.Errorf("resolver is required")
	}

	tempDir := opts.TempDir
	if tempDir == "" {
		tempDir = filepath.Join(opts.OutputDir, ".release")
	}
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return nil, err
	}

	publishedArtifacts := make([]PublishedAsset, 0, len(opts.Artifacts))
	uploads := make([]UploadAsset, 0, len(opts.Artifacts)+2)
	for _, artifact := range opts.Artifacts {
		sourcePath := filepath.Join(opts.OutputDir, filepath.FromSlash(artifact.Path))
		info, err := os.Stat(sourcePath)
		if err != nil {
			return nil, err
		}

		published := PublishedAsset{
			LogicalPath: artifact.Path,
			OS:          artifact.OS,
			Arch:        artifact.Arch,
			Format:      artifact.Format,
			Checksum:    artifact.Checksum,
			Size:        artifact.Size,
			Metadata:    cloneMetadata(artifact.Metadata),
		}

		if info.IsDir() {
			zipName := artifactAssetName(opts.App.Name, opts.Version, artifact, ".zip")
			zipPath := filepath.Join(tempDir, zipName)
			if err := zipDirectory(sourcePath, zipPath); err != nil {
				return nil, err
			}
			published.AssetName = zipName
			published.SourcePath = zipPath
			published.Transport = "archive"
			uploads = append(uploads, UploadAsset{
				Name:        zipName,
				Path:        zipPath,
				ContentType: "application/zip",
			})
		} else {
			ext := filepath.Ext(sourcePath)
			assetName := artifactAssetName(opts.App.Name, opts.Version, artifact, ext)
			published.AssetName = assetName
			published.SourcePath = sourcePath
			published.Transport = "file"
			uploads = append(uploads, UploadAsset{
				Name:        assetName,
				Path:        sourcePath,
				ContentType: contentTypeForName(assetName),
			})
		}

		checksum, size, err := uploadedAssetMetadata(published.SourcePath)
		if err != nil {
			return nil, err
		}
		published.Checksum = checksum
		published.Size = size

		publishedArtifacts = append(publishedArtifacts, published)
	}

	sort.Slice(publishedArtifacts, func(i, j int) bool {
		return publishedArtifacts[i].LogicalPath < publishedArtifacts[j].LogicalPath
	})

	manifest := &Manifest{
		SchemaVersion: 1,
		App: ManifestApp{
			Name:       opts.App.Name,
			Identifier: opts.App.Identifier,
		},
		Release: ManifestRelease{
			Tag:      opts.Tag,
			Version:  opts.Version,
			Provider: opts.Resolver.Provider(),
		},
		GeneratedAt: nowUTC(),
		Artifacts:   make([]ManifestArtifact, 0, len(publishedArtifacts)),
	}

	for _, artifact := range publishedArtifacts {
		manifest.Artifacts = append(manifest.Artifacts, ManifestArtifact{
			Path:      artifact.LogicalPath,
			AssetName: artifact.AssetName,
			OS:        artifact.OS,
			Arch:      artifact.Arch,
			Format:    artifact.Format,
			Transport: artifact.Transport,
			Checksum:  artifact.Checksum,
			Size:      artifact.Size,
			URL:       opts.Resolver.ArtifactURL(opts.Tag, artifact.AssetName),
			Metadata:  cloneMetadata(artifact.Metadata),
		})
	}

	manifestPath := filepath.Join(opts.OutputDir, ManifestAssetName)
	if opts.Delta != nil && opts.Delta.ManifestPath != "" {
		manifest.Delta = &ManifestDelta{ManifestURL: opts.Resolver.DeltaManifestURL(opts.Tag)}
	}
	if err := ValidateManifest(manifest); err != nil {
		return nil, err
	}
	if err := WriteManifest(manifest, manifestPath); err != nil {
		return nil, err
	}
	uploads = append(uploads, UploadAsset{
		Name:        ManifestAssetName,
		Path:        manifestPath,
		ContentType: "application/json",
	})

	bundle := &Bundle{
		Manifest:     manifest,
		ManifestPath: manifestPath,
		Artifacts:    publishedArtifacts,
		Uploads:      uploads,
	}

	if opts.Delta != nil && opts.Delta.ManifestPath != "" {
		deltaManifestPath, deltaUploads, err := preparePublishedDeltaAssets(opts, tempDir)
		if err != nil {
			return nil, err
		}
		bundle.DeltaManifestPath = deltaManifestPath
		bundle.Uploads = append(bundle.Uploads, deltaUploads...)
	}

	return bundle, nil
}

func preparePublishedDeltaAssets(opts BundleOptions, tempDir string) (string, []UploadAsset, error) {
	patchURLs := make(map[string]string, len(opts.Delta.Generated))
	uploads := make([]UploadAsset, 0, len(opts.Delta.Generated)+1)
	for _, generated := range opts.Delta.Generated {
		artifact, ok := findArtifact(opts.Artifacts, generated.Artifact)
		if !ok {
			return "", nil, fmt.Errorf("missing build artifact for delta %s", generated.Artifact)
		}
		assetName := deltaAssetName(opts.App.Name, opts.Version, generated.FromVersion, artifact)
		patchURLs[filepath.Clean(generated.Patch)] = opts.Resolver.ArtifactURL(opts.Tag, assetName)
		uploads = append(uploads, UploadAsset{
			Name:        assetName,
			Path:        generated.Patch,
			ContentType: "application/octet-stream",
		})
	}

	manifest, err := publishedDeltaManifest(opts.OutputDir, opts.Delta.Generated, patchURLs)
	if err != nil {
		return "", nil, err
	}
	deltaManifestPath := filepath.Join(tempDir, DeltaManifestAssetName)
	if err := delta.WriteManifest(manifest, deltaManifestPath); err != nil {
		return "", nil, err
	}
	uploads = append(uploads, UploadAsset{
		Name:        DeltaManifestAssetName,
		Path:        deltaManifestPath,
		ContentType: "application/json",
	})
	return deltaManifestPath, uploads, nil
}

func publishedDeltaManifest(outputDir string, generated []delta.Generated, patchURLs map[string]string) (*delta.PatchManifest, error) {
	manifest := delta.BuildManifest(outputDir, generated)
	for i := range manifest.Patches {
		key := filepath.Clean(generated[i].Patch)
		if url, ok := patchURLs[key]; ok {
			manifest.Patches[i].Patch = url
		}
	}
	if err := delta.ValidateManifest(manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

func findArtifact(artifacts []build.Artifact, logicalPath string) (build.Artifact, bool) {
	for _, artifact := range artifacts {
		if artifact.Path == logicalPath {
			return artifact, true
		}
	}
	return build.Artifact{}, false
}

func artifactAssetName(appName, version string, artifact build.Artifact, ext string) string {
	return canonicalName(appName, version, artifact.OS, artifact.Arch, artifact.Format, ext)
}

func deltaAssetName(appName, version, fromVersion string, artifact build.Artifact) string {
	return fmt.Sprintf(
		"%s-%s-delta-from-%s-%s-%s-%s.patch",
		sanitizeSegment(appName),
		sanitizeSegment(version),
		sanitizeSegment(fromVersion),
		sanitizeSegment(artifact.OS),
		sanitizeSegment(artifact.Arch),
		sanitizeSegment(artifact.Format),
	)
}

func canonicalName(parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		filtered = append(filtered, sanitizeSegment(part))
	}
	if len(filtered) == 0 {
		return ""
	}
	base := strings.Join(filtered[:len(filtered)-1], "-")
	last := filtered[len(filtered)-1]
	if strings.HasPrefix(last, ".") {
		return base + last
	}
	if base == "" {
		return last
	}
	return base + "-" + last
}

func sanitizeSegment(value string) string {
	var out strings.Builder
	for _, r := range value {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			out.WriteRune(r)
		case r == '.' || r == '_' || r == '-':
			out.WriteRune(r)
		default:
			out.WriteByte('-')
		}
	}
	text := strings.Trim(out.String(), "-")
	for strings.Contains(text, "--") {
		text = strings.ReplaceAll(text, "--", "-")
	}
	if text == "" {
		return "asset"
	}
	return text
}

func uploadedAssetMetadata(path string) (string, int64, error) {
	checksum, err := build.ComputeChecksum(path)
	if err != nil {
		return "", 0, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", 0, err
	}
	return "sha256:" + checksum, info.Size(), nil
}

func zipDirectory(srcDir, dstPath string) error {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := zip.NewWriter(file)
	defer writer.Close()

	if err := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == srcDir {
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = rel
		if d.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}
		if info.Mode()&os.ModeSymlink != 0 {
			header.Method = zip.Store
		}
		entry, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return nil
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			_, err = io.WriteString(entry, target)
			return err
		default:
			in, err := os.Open(path)
			if err != nil {
				return err
			}
			defer in.Close()
			_, err = io.Copy(entry, in)
			return err
		}
	}); err != nil {
		return err
	}

	if err := writer.Close(); err != nil {
		return err
	}
	return file.Close()
}

func contentTypeForName(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".json":
		return "application/json"
	case ".zip":
		return "application/zip"
	default:
		return "application/octet-stream"
	}
}

func cloneMetadata(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

var nowUTC = func() time.Time {
	return time.Now().UTC()
}
