package frontend

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	internalexec "github.com/Eriyc/wailsrel/internal/exec"
	"github.com/Eriyc/wailsrel/pkg/build"
)

type BundleManifest struct {
	BundleVersion string `json:"bundle_version,omitempty"`
	Version       string `json:"version,omitempty"`
	CompatID      string `json:"compat_id,omitempty"`
	CompatVersion int    `json:"compat_version,omitempty"`
	Channel       string `json:"channel"`
	MinNativeVer  string `json:"min_native_version,omitempty"`
	Checksum      string `json:"checksum,omitempty"`
	Timestamp     string `json:"timestamp,omitempty"`
}

type Bundler interface {
	BuildBundle(ctx context.Context, opts BundleOpts) (*BundleArtifact, error)
}

type BundleOpts struct {
	WorkDir       string
	OutputDir     string
	OutputPath    string
	BuildCommand  string
	BuildDir      string
	BindingsDir   string
	CompatVer     string
	CompatVersion int
	Channel       string
	Version       string
}

type BundleArtifact struct {
	Path     string
	Manifest BundleManifest
	Size     int64
}

type DefaultBundler struct{}

func BuildBundle(ctx context.Context, opts BundleOpts) (*BundleArtifact, error) {
	return DefaultBundler{}.BuildBundle(ctx, opts)
}

func (DefaultBundler) BuildBundle(ctx context.Context, opts BundleOpts) (*BundleArtifact, error) {
	workDir := firstNonEmptyPath(opts.WorkDir, ".")
	buildDir := opts.BuildDir
	if !filepath.IsAbs(buildDir) {
		buildDir = filepath.Join(workDir, buildDir)
	}
	outputDir := opts.OutputDir
	if outputDir == "" {
		outputDir = workDir
	}
	if !filepath.IsAbs(outputDir) {
		outputDir = filepath.Join(workDir, outputDir)
	}

	if strings.TrimSpace(opts.BuildCommand) == "" {
		return nil, fmt.Errorf("build command is required")
	}
	if strings.TrimSpace(opts.Channel) == "" {
		return nil, fmt.Errorf("channel is required")
	}
	if strings.TrimSpace(opts.Version) == "" {
		return nil, fmt.Errorf("version is required")
	}

	if _, err := runBuildCommand(ctx, workDir, opts.BuildCommand); err != nil {
		return nil, err
	}

	buildInfo, err := os.Stat(buildDir)
	if err != nil {
		return nil, err
	}
	if !buildInfo.IsDir() {
		return nil, fmt.Errorf("frontend build dir %s is not a directory", buildDir)
	}

	compatID := strings.TrimSpace(opts.CompatVer)
	compatVersion := opts.CompatVersion
	if compatID == "" && compatVersion > 0 {
		compatID = strconv.Itoa(compatVersion)
	}
	if compatVersion == 0 && compatID != "" {
		if parsed, err := strconv.Atoi(compatID); err == nil {
			compatVersion = parsed
		}
	}

	checksum, err := build.ComputeChecksum(buildDir)
	if err != nil {
		return nil, err
	}

	manifest := BundleManifest{
		BundleVersion: opts.Version,
		Version:       opts.Version,
		CompatID:      compatID,
		CompatVersion: compatVersion,
		Channel:       opts.Channel,
		Checksum:      "sha256:" + checksum,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
	}

	outputPath := opts.OutputPath
	if outputPath == "" {
		outputPath = filepath.Join(outputDir, fmt.Sprintf("frontend-%s-%s.zip", opts.Channel, opts.Version))
	}
	if !filepath.IsAbs(outputPath) {
		outputPath = filepath.Join(workDir, outputPath)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return nil, err
	}
	if err := writeBundleArchive(outputPath, buildDir, manifest); err != nil {
		return nil, err
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		return nil, err
	}
	return &BundleArtifact{
		Path:     outputPath,
		Manifest: manifest,
		Size:     info.Size(),
	}, nil
}

func writeBundleArchive(outputPath, buildDir string, manifest BundleManifest) error {
	file, err := os.OpenFile(outputPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	zw := zip.NewWriter(file)
	defer zw.Close()

	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	manifestData = append(manifestData, '\n')
	if err := writeZipFile(zw, "bundle.json", manifestData, 0o644); err != nil {
		return err
	}

	err = filepath.WalkDir(buildDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(buildDir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		return writeZipFile(zw, filepath.ToSlash(rel), data, info.Mode())
	})
	if err != nil {
		return err
	}

	if err := zw.Close(); err != nil {
		return err
	}
	return file.Close()
}

func writeZipFile(zw *zip.Writer, name string, data []byte, mode fs.FileMode) error {
	header := &zip.FileHeader{
		Name:   filepath.ToSlash(name),
		Method: zip.Deflate,
	}
	header.SetMode(mode)
	header.Modified = time.Now()

	writer, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

func runBuildCommand(ctx context.Context, workDir, command string) (*internalexec.Result, error) {
	var name string
	var args []string
	if runtime.GOOS == "windows" {
		name = "cmd"
		args = []string{"/C", command}
	} else {
		name = "sh"
		args = []string{"-c", command}
	}

	result, err := internalexec.Run(ctx, name, args, internalexec.Options{
		Dir: workDir,
	})
	if err != nil {
		message := strings.TrimSpace(resultOutput(result))
		if message != "" {
			return result, fmt.Errorf("%w: %s", err, message)
		}
		return result, err
	}
	return result, nil
}

func resultOutput(result *internalexec.Result) string {
	if result == nil {
		return ""
	}

	parts := make([]string, 0, 2)
	if text := strings.TrimSpace(result.Stdout); text != "" {
		parts = append(parts, text)
	}
	if text := strings.TrimSpace(result.Stderr); text != "" {
		parts = append(parts, text)
	}
	return strings.Join(parts, "\n")
}

func firstNonEmptyPath(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func readBundleManifestFromZip(zipPath string) (*BundleManifest, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return bundleManifestFromZipFiles(reader.File)
}

func bundleManifestFromZipFiles(files []*zip.File) (*BundleManifest, error) {
	for _, file := range files {
		if file.Name != "bundle.json" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()

		var manifest BundleManifest
		if err := json.NewDecoder(rc).Decode(&manifest); err != nil {
			return nil, err
		}
		return &manifest, nil
	}
	return nil, fmt.Errorf("bundle.json not found in archive")
}

func loadBundleManifest(path string) (*BundleManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest BundleManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func writeBundleManifest(path string, manifest BundleManifest) error {
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

func extractBundleArchive(zipPath, destination string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer reader.Close()

	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}

	for _, file := range reader.File {
		target := filepath.Join(destination, filepath.FromSlash(file.Name))
		cleanTarget := filepath.Clean(target)
		if cleanTarget != destination && !strings.HasPrefix(cleanTarget, destination+string(os.PathSeparator)) {
			return fmt.Errorf("invalid bundle entry %q", file.Name)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(cleanTarget, 0o755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(cleanTarget), 0o755); err != nil {
			return err
		}

		rc, err := file.Open()
		if err != nil {
			return err
		}

		out, err := os.OpenFile(cleanTarget, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, file.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return err
		}
		if err := out.Close(); err != nil {
			rc.Close()
			return err
		}
		if err := rc.Close(); err != nil {
			return err
		}
	}

	return nil
}
