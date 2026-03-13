package sign

import (
	"path/filepath"
	"strings"
)

func isAppBundle(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".app")
}

func isDMG(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".dmg")
}

func isWindowsBinary(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".exe")
}
