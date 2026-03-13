package frontend

import (
	"errors"
	"io/fs"
	"path"
	"sort"
	"strings"
)

// LayeredFS prefers files from the override filesystem and falls back to the embedded filesystem.
type LayeredFS struct {
	layers []fs.FS
}

func NewLayeredFS(override, embedded fs.FS) *LayeredFS {
	layers := make([]fs.FS, 0, 2)
	if override != nil {
		layers = append(layers, override)
	}
	if embedded != nil {
		layers = append(layers, embedded)
	}
	return &LayeredFS{layers: layers}
}

func (l *LayeredFS) Open(name string) (fs.File, error) {
	return openLayeredFile(l.layers, name)
}

func (l *LayeredFS) ReadFile(name string) ([]byte, error) {
	normalized, err := normalizeLayeredPath(name)
	if err != nil {
		return nil, err
	}

	var firstErr error
	for _, layer := range l.layers {
		if layer == nil {
			continue
		}
		data, err := fs.ReadFile(layer, normalized)
		if err == nil {
			return data, nil
		}
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return nil, fs.ErrNotExist
}

func (l *LayeredFS) ReadDir(name string) ([]fs.DirEntry, error) {
	normalized, err := normalizeLayeredPath(name)
	if err != nil {
		return nil, err
	}

	merged := make(map[string]fs.DirEntry)
	var found bool
	var firstErr error
	for _, layer := range l.layers {
		if layer == nil {
			continue
		}
		entries, err := fs.ReadDir(layer, normalized)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		found = true
		for _, entry := range entries {
			if _, ok := merged[entry.Name()]; !ok {
				merged[entry.Name()] = entry
			}
		}
	}
	if !found {
		if firstErr != nil {
			return nil, firstErr
		}
		return nil, fs.ErrNotExist
	}

	names := make([]string, 0, len(merged))
	for name := range merged {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]fs.DirEntry, 0, len(names))
	for _, name := range names {
		out = append(out, merged[name])
	}
	return out, nil
}

func (l *LayeredFS) Stat(name string) (fs.FileInfo, error) {
	normalized, err := normalizeLayeredPath(name)
	if err != nil {
		return nil, err
	}

	var firstErr error
	for _, layer := range l.layers {
		if layer == nil {
			continue
		}
		info, err := fs.Stat(layer, normalized)
		if err == nil {
			return info, nil
		}
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return nil, fs.ErrNotExist
}

func openLayeredFile(layers []fs.FS, name string) (fs.File, error) {
	normalized, err := normalizeLayeredPath(name)
	if err != nil {
		return nil, err
	}

	var firstErr error
	for _, layer := range layers {
		if layer == nil {
			continue
		}
		file, err := layer.Open(normalized)
		if err == nil {
			return file, nil
		}
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return nil, fs.ErrNotExist
}

func normalizeLayeredPath(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" || trimmed == "." || trimmed == "/" {
		return ".", nil
	}

	clean := path.Clean(strings.TrimPrefix(trimmed, "/"))
	if clean == "." {
		return ".", nil
	}
	if !fs.ValidPath(clean) {
		return "", fs.ErrInvalid
	}
	return clean, nil
}
