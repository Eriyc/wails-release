package frontend

import (
	"io/fs"
	"os"
)

type RuntimeFS struct {
	manager  *BundleManager
	embedded fs.FS
}

func NewRuntimeFS(manager *BundleManager, embedded fs.FS) fs.FS {
	return &RuntimeFS{
		manager:  manager,
		embedded: embedded,
	}
}

func (r *RuntimeFS) Open(name string) (fs.File, error) {
	layers, unlock := r.snapshotLayers()
	if unlock != nil {
		defer unlock()
	}
	return openLayeredFile(layers, name)
}

func (r *RuntimeFS) ReadFile(name string) ([]byte, error) {
	layers, unlock := r.snapshotLayers()
	if unlock != nil {
		defer unlock()
	}
	return (&LayeredFS{layers: layers}).ReadFile(name)
}

func (r *RuntimeFS) ReadDir(name string) ([]fs.DirEntry, error) {
	layers, unlock := r.snapshotLayers()
	if unlock != nil {
		defer unlock()
	}
	return (&LayeredFS{layers: layers}).ReadDir(name)
}

func (r *RuntimeFS) Stat(name string) (fs.FileInfo, error) {
	layers, unlock := r.snapshotLayers()
	if unlock != nil {
		defer unlock()
	}
	return (&LayeredFS{layers: layers}).Stat(name)
}

func (r *RuntimeFS) snapshotLayers() ([]fs.FS, func()) {
	layers := make([]fs.FS, 0, 2)
	if r.manager == nil {
		if r.embedded != nil {
			layers = append(layers, r.embedded)
		}
		return layers, nil
	}

	r.manager.mu.RLock()
	dir, _, _ := r.manager.resolveEffectiveLocked()
	if dir != "" {
		layers = append(layers, osDirFS(dir))
	}
	if r.embedded != nil {
		layers = append(layers, r.embedded)
	}
	return layers, r.manager.mu.RUnlock
}

func osDirFS(path string) fs.FS {
	return os.DirFS(path)
}
