package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorReportsSigningCredentialProblems(t *testing.T) {
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
	for _, name := range []string{"go", "git", "wails3"} {
		writeDoctorTool(t, filepath.Join(toolsDir, name))
	}

	t.Setenv("PATH", toolsDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cmd := NewRootCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--config", configPath, "--json", "doctor"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected doctor to fail")
	}

	output := stdout.String()
	if !strings.Contains(output, `"name": "makensis"`) {
		t.Fatalf("expected missing tool check in output, got %q", output)
	}
	if !strings.Contains(output, `"found": false`) {
		t.Fatalf("expected missing tool result in output, got %q", output)
	}
}

func writeDoctorTool(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write tool %s: %v", path, err)
	}
}
