package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMainPrintsUserFacingErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script stubs are not supported on windows")
	}

	repo := t.TempDir()
	configPath := filepath.Join(repo, "wailsrel.yaml")
	if err := os.WriteFile(configPath, []byte(`
schema: 2
app:
  name: "Test App"
  identifier: "com.example.test"
targets:
  - id: windows-amd64
    os: windows
    arch: amd64
    build:
      argv: ["task", "build"]
      requires: ["task", "wails3", "makensis"]
    artifacts:
      - format: exe
        path: "bin/Test App.exe"
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	toolsDir := filepath.Join(repo, "tools")
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatalf("mkdir tools: %v", err)
	}
	for _, name := range []string{"task", "wails3"} {
		if err := os.WriteFile(filepath.Join(toolsDir, name), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatalf("write tool %s: %v", name, err)
		}
	}

	cmd := exec.Command("go", "run", "./cmd/wailsrel", "--config", configPath, "doctor")
	cmd.Dir = filepath.Join("..", "..")
	cmd.Env = append(os.Environ(), "PATH="+toolsDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected command failure")
	}

	text := string(output)
	if !strings.Contains(text, "makensis") {
		t.Fatalf("expected missing tool in output, got %q", text)
	}
}
