package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDoctorReportsSigningCredentialProblems(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script stubs are not supported on windows")
	}

	repo := t.TempDir()
	configPath := filepath.Join(repo, "wailsrel.yaml")
	if err := os.WriteFile(configPath, []byte(`
app:
  name: "Test App"
  identifier: "com.example.test"
targets:
  - os: windows
    arch: [amd64]
    output_formats: [exe]
    sign:
      provider: azure
      endpoint: "https://account.codesigning.azure.net"
      account: "account"
      profile: "profile"
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	toolsDir := filepath.Join(repo, "tools")
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatalf("mkdir tools: %v", err)
	}
	for _, name := range []string{"go", "git", "wails3", "signtool"} {
		writeDoctorTool(t, filepath.Join(toolsDir, name))
	}

	dlibPath := filepath.Join(repo, "Azure.CodeSigning.Dlib.dll")
	if err := os.WriteFile(dlibPath, []byte("dlib"), 0o644); err != nil {
		t.Fatalf("write dlib: %v", err)
	}

	t.Setenv("PATH", toolsDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AZURE_TRUSTED_SIGNING_DLIB", dlibPath)
	t.Setenv("AZURE_TENANT_ID", "")
	t.Setenv("AZURE_CLIENT_ID", "")
	t.Setenv("AZURE_CLIENT_SECRET", "")

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
	if !strings.Contains(output, `"name": "sign windows/azure"`) {
		t.Fatalf("expected signing check in output, got %q", output)
	}
	if !strings.Contains(output, "AZURE_TENANT_ID is required") {
		t.Fatalf("expected credential error in output, got %q", output)
	}
}

func writeDoctorTool(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write tool %s: %v", path, err)
	}
}
