package build

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func ComputeChecksum(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}

	hash := sha256.New()
	if info.IsDir() {
		if err := hashDirectory(hash, path); err != nil {
			return "", err
		}
	} else {
		if err := hashFile(hash, path); err != nil {
			return "", err
		}
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func hashDirectory(w io.Writer, root string) error {
	var paths []string
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		paths = append(paths, path)
		return nil
	}); err != nil {
		return err
	}

	sort.Strings(paths)
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		if _, err := io.WriteString(w, filepath.ToSlash(rel)); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "\n"); err != nil {
			return err
		}
		if _, err := io.WriteString(w, info.Mode().String()); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "\n"); err != nil {
			return err
		}

		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if _, err := io.WriteString(w, target); err != nil {
				return err
			}
			if _, err := io.WriteString(w, "\n"); err != nil {
				return err
			}
			continue
		}

		if info.IsDir() {
			continue
		}

		if err := hashFile(w, path); err != nil {
			return err
		}
	}

	return nil
}

func hashFile(w io.Writer, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(w, file)
	return err
}

func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return copyDir(src, dst)
	}

	return copyFile(src, dst, info.Mode())
}

func copyDir(src, dst string) error {
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

		info, err := os.Lstat(path)
		if err != nil {
			return err
		}

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}

		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			return os.Symlink(linkTarget, target)
		}

		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return out.Close()
}

func pathSize(path string) (int64, error) {
	var size int64
	err := filepath.WalkDir(path, func(current string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		size += info.Size()
		return nil
	})
	return size, err
}

func writeChecksumFile(path, checksum string) error {
	content := fmt.Sprintf("%s  %s\n", checksum, filepath.Base(path))
	return os.WriteFile(path+".sha256", []byte(content), 0o644)
}

func zipPath(srcPath, dstPath string) error {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}

	file, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	zw := zip.NewWriter(file)
	defer zw.Close()

	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		return err
	}

	baseName := filepath.Base(srcPath)
	if srcInfo.IsDir() {
		err = filepath.WalkDir(srcPath, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}

			rel, err := filepath.Rel(srcPath, path)
			if err != nil {
				return err
			}

			return addFileToZip(zw, path, filepath.ToSlash(filepath.Join(baseName, rel)))
		})
		if err != nil {
			return err
		}
	} else {
		if err := addFileToZip(zw, srcPath, baseName); err != nil {
			return err
		}
	}

	return zw.Close()
}

func addFileToZip(zw *zip.Writer, srcPath, name string) error {
	info, err := os.Stat(srcPath)
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = filepath.ToSlash(name)
	header.Method = zip.Deflate

	writer, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}

	file, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(writer, file)
	return err
}

func newestMatching(paths []string) (string, error) {
	var (
		bestPath string
		bestInfo os.FileInfo
	)

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", err
		}

		if bestPath == "" || info.ModTime().After(bestInfo.ModTime()) {
			bestPath = path
			bestInfo = info
		}
	}

	if bestPath == "" {
		return "", fs.ErrNotExist
	}

	return bestPath, nil
}

func findArtifact(root string, patterns ...string) (string, error) {
	var matches []string
	for _, pattern := range patterns {
		found, err := filepath.Glob(filepath.Join(root, pattern))
		if err != nil {
			return "", err
		}
		matches = append(matches, found...)
	}

	sort.Strings(matches)
	return newestMatching(matches)
}

func normalizeArtifactPath(path string) string {
	return filepath.ToSlash(strings.TrimPrefix(path, string(filepath.Separator)))
}

func requirePath(path string) error {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("expected build artifact %s to exist", path)
		}
		return err
	}
	return nil
}
