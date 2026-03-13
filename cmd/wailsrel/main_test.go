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

	artifactPath := filepath.Join(repo, "dist", "MyApp.exe")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatalf("mkdir dist: %v", err)
	}
	if err := os.WriteFile(artifactPath, []byte("binary"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	toolsDir := filepath.Join(repo, "tools")
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatalf("mkdir tools: %v", err)
	}
	if err := os.WriteFile(filepath.Join(toolsDir, "signtool"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write signtool: %v", err)
	}

	dlibPath := filepath.Join(repo, "Azure.CodeSigning.Dlib.dll")
	if err := os.WriteFile(dlibPath, []byte("dlib"), 0o644); err != nil {
		t.Fatalf("write dlib: %v", err)
	}

	cmd := exec.Command("go", "run", "./cmd/wailsrel", "--config", configPath, "sign", artifactPath)
	cmd.Dir = filepath.Join("..", "..")
	cmd.Env = append(os.Environ(),
		"PATH="+toolsDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"AZURE_TRUSTED_SIGNING_DLIB="+dlibPath,
		"AZURE_TENANT_ID=",
		"AZURE_CLIENT_ID=",
		"AZURE_CLIENT_SECRET=",
	)

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected command failure")
	}

	text := string(output)
	if !strings.Contains(text, "AZURE_TENANT_ID is required") {
		t.Fatalf("expected credential error in output, got %q", text)
	}
}
