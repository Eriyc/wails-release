package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBumpMinorCreatesAnnotatedTag(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repo, configPath := newCLITestRepo(t)
	gitRun(t, repo, "tag", "v1.2.3")
	writeRepoFile(t, repo, "notes.txt", "release notes\n")
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-m", "feat: add release notes")

	cmd := NewRootCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--config", configPath, "bump", "minor"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute bump: %v", err)
	}

	tagList := gitRun(t, repo, "tag", "--list")
	if !strings.Contains(tagList, "v1.3.0") {
		t.Fatalf("expected new tag, got %q", tagList)
	}

	annotation := gitRun(t, repo, "tag", "-n99", "v1.3.0")
	if !strings.Contains(annotation, "Release 1.3.0") {
		t.Fatalf("expected changelog annotation, got %q", annotation)
	}
}

func TestBumpPatchPrerelease(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repo, configPath := newCLITestRepo(t)
	gitRun(t, repo, "tag", "v1.2.3")
	writeRepoFile(t, repo, "notes.txt", "release notes\n")
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-m", "fix: prepare prerelease")

	cmd := NewRootCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--config", configPath, "bump", "patch", "--prerelease"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute bump prerelease: %v", err)
	}

	tagList := gitRun(t, repo, "tag", "--list")
	if !strings.Contains(tagList, "v1.2.4-beta.1") {
		t.Fatalf("expected prerelease tag, got %q", tagList)
	}
}

func TestBumpRejectsDirtyRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repo, configPath := newCLITestRepo(t)
	writeRepoFile(t, repo, "dirty.txt", "uncommitted\n")

	cmd := NewRootCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--config", configPath, "bump", "patch"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected dirty repo error")
	}
	if !strings.Contains(err.Error(), "working tree is dirty") {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestBumpPushesTag(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	baseDir := t.TempDir()
	remote := filepath.Join(baseDir, "remote.git")
	gitRun(t, "", "init", "--bare", remote)

	repo, configPath := newCLITestRepo(t)
	gitRun(t, repo, "remote", "add", "origin", remote)
	gitRun(t, repo, "tag", "v1.2.3")
	writeRepoFile(t, repo, "notes.txt", "release notes\n")
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-m", "feat: push release tag")

	cmd := NewRootCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--config", configPath, "bump", "minor", "--push"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute bump push: %v", err)
	}

	remoteTags := gitRun(t, "", "--git-dir", remote, "tag", "--list")
	if !strings.Contains(remoteTags, "v1.3.0") {
		t.Fatalf("expected remote tag, got %q", remoteTags)
	}
}

func newCLITestRepo(t *testing.T) (string, string) {
	t.Helper()

	repo := t.TempDir()
	gitRun(t, repo, "init")
	gitRun(t, repo, "config", "user.name", "Test User")
	gitRun(t, repo, "config", "user.email", "test@example.com")

	configPath := filepath.Join(repo, "wailsrel.yaml")
	if err := os.WriteFile(configPath, []byte(testConfig()), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	writeRepoFile(t, repo, "README.md", "hello\n")
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-m", "chore: initial commit")

	return repo, configPath
}

func testConfig() string {
	return `
app:
  name: "Test App"
  identifier: "com.example.test"
targets:
  - os: darwin
    arch: [amd64]
    output_formats: [app]
`
}

func writeRepoFile(t *testing.T, dir, name, content string) {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}
