package artifact

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

type OutputResult struct {
	ManifestPath string
	Artifacts    []string
}

func PrepareOutputDir(path string, clean bool) error {
	if clean {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return os.MkdirAll(path, 0o755)
}

func PrepareUploadDir(artifacts []string, manifest string, outDir string) error {
	if err := os.RemoveAll(outDir); err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	for _, path := range artifacts {
		if err := copyPath(path, filepath.Join(outDir, filepath.Base(path))); err != nil {
			return err
		}
	}

	if manifest != "" {
		if err := copyPath(manifest, filepath.Join(outDir, filepath.Base(manifest))); err != nil {
			return err
		}
	}

	return nil
}

func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			rel, err := filepath.Rel(src, path)
			if err != nil {
				return err
			}

			target := dst
			if rel != "." {
				target = filepath.Join(dst, rel)
			}

			if d.IsDir() {
				return os.MkdirAll(target, 0o755)
			}

			return copyFile(path, target)
		})
	}

	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return out.Close()
}
