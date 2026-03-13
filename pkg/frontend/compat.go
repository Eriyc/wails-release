package frontend

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type CompatSnapshot struct {
	CompatVersion int       `json:"compat_version"`
	BindingsHash  string    `json:"bindings_hash"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func ComputeCompatHash(bindings any) (string, error) {
	paths, readPath, err := compatHasher(bindings)
	if err != nil {
		return "", err
	}

	hash := sha256.New()
	for _, path := range paths {
		info, contents, err := readPath(path)
		if err != nil {
			return "", err
		}
		rel := filepath.ToSlash(path)

		if _, err := io.WriteString(hash, rel); err != nil {
			return "", err
		}
		if _, err := io.WriteString(hash, "\n"); err != nil {
			return "", err
		}
		if _, err := io.WriteString(hash, info.Mode().String()); err != nil {
			return "", err
		}
		if _, err := io.WriteString(hash, "\n"); err != nil {
			return "", err
		}
		if info.IsDir() {
			continue
		}
		if _, err := hash.Write(contents); err != nil {
			return "", err
		}
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func compatHasher(bindings any) ([]string, func(string) (fs.FileInfo, []byte, error), error) {
	switch typed := bindings.(type) {
	case string:
		root := filepath.Clean(strings.TrimSpace(typed))
		if root == "" {
			return nil, nil, fmt.Errorf("bindings directory is required")
		}
		info, err := os.Stat(root)
		if err != nil {
			return nil, nil, err
		}
		if !info.IsDir() {
			return nil, nil, fmt.Errorf("bindings path %s is not a directory", root)
		}

		paths := make([]string, 0)
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if path == root {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			paths = append(paths, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
		sort.Strings(paths)

		return paths, func(path string) (fs.FileInfo, []byte, error) {
			fullPath := filepath.Join(root, filepath.FromSlash(path))
			info, err := os.Lstat(fullPath)
			if err != nil {
				return nil, nil, err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				target, err := os.Readlink(fullPath)
				if err != nil {
					return nil, nil, err
				}
				return info, []byte(target + "\n"), nil
			}
			if info.IsDir() {
				return info, nil, nil
			}
			data, err := os.ReadFile(fullPath)
			if err != nil {
				return nil, nil, err
			}
			return info, data, nil
		}, nil
	case fs.FS:
		info, err := fs.Stat(typed, ".")
		if err != nil {
			return nil, nil, err
		}
		if !info.IsDir() {
			return nil, nil, fmt.Errorf("bindings fs root is not a directory")
		}

		paths := make([]string, 0)
		err = fs.WalkDir(typed, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if path == "." {
				return nil
			}
			paths = append(paths, filepath.ToSlash(path))
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
		sort.Strings(paths)

		return paths, func(path string) (fs.FileInfo, []byte, error) {
			info, err := fs.Stat(typed, path)
			if err != nil {
				return nil, nil, err
			}
			if info.IsDir() {
				return info, nil, nil
			}
			data, err := fs.ReadFile(typed, path)
			if err != nil {
				return nil, nil, err
			}
			return info, data, nil
		}, nil
	default:
		return nil, nil, fmt.Errorf("unsupported bindings source %T", bindings)
	}
}

func LoadCompatSnapshot(path string) (*CompatSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var snapshot CompatSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func WriteCompatSnapshot(path string, snapshot CompatSnapshot) error {
	snapshot.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func CheckCompatSnapshot(path string, compatVersion int, bindingsHash string) (string, error) {
	snapshot, err := LoadCompatSnapshot(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if snapshot.BindingsHash == "" || snapshot.BindingsHash == bindingsHash {
		return "", nil
	}
	if snapshot.CompatVersion == compatVersion {
		return fmt.Sprintf(
			"frontend bindings changed but frontend.compat_version is still %d; bump compat_version if the Go↔JS contract changed",
			compatVersion,
		), nil
	}
	return "", nil
}

func CheckCompat(bundle BundleManifest, nativeCompat string) error {
	bundleCompat := strings.TrimSpace(bundle.CompatID)
	if bundleCompat == "" && bundle.CompatVersion > 0 {
		bundleCompat = strconv.Itoa(bundle.CompatVersion)
	}
	nativeCompat = strings.TrimSpace(nativeCompat)

	if bundleCompat == "" || nativeCompat == "" {
		return nil
	}
	if bundleCompat != nativeCompat {
		return fmt.Errorf("frontend bundle compat %q does not match native compat %q", bundleCompat, nativeCompat)
	}
	return nil
}
