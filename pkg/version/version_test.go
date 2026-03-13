package version

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAndCompare(t *testing.T) {
	release, err := Parse("1.2.3")
	if err != nil {
		t.Fatalf("parse release: %v", err)
	}

	prerelease, err := Parse("1.2.3-beta.1+build456")
	if err != nil {
		t.Fatalf("parse prerelease: %v", err)
	}

	if release.Compare(prerelease) <= 0 {
		t.Fatalf("expected release to be newer than prerelease")
	}
	if prerelease.Metadata != "build456" {
		t.Fatalf("unexpected metadata %q", prerelease.Metadata)
	}
}

func TestParseRejectsInvalidNumericPrerelease(t *testing.T) {
	if _, err := Parse("1.2.3-beta.01"); err == nil {
		t.Fatal("expected invalid prerelease to fail")
	}
}

func TestBump(t *testing.T) {
	base := Version{Major: 1, Minor: 2, Patch: 3}

	if got := base.Bump(BumpPatch, "beta.{n}"); got.String() != "1.2.4" {
		t.Fatalf("unexpected patch bump %s", got.String())
	}
	if got := base.Bump(BumpMinor, "beta.{n}"); got.String() != "1.3.0" {
		t.Fatalf("unexpected minor bump %s", got.String())
	}
	if got := base.Bump(BumpMajor, "beta.{n}"); got.String() != "2.0.0" {
		t.Fatalf("unexpected major bump %s", got.String())
	}
	if got := base.Bump(BumpPrerelease, "beta.{n}"); got.String() != "1.2.3-beta.1" {
		t.Fatalf("unexpected prerelease bump %s", got.String())
	}
	if got := gotPrerelease(base).Bump(BumpPrerelease, "beta.{n}"); got.String() != "1.2.3-beta.2" {
		t.Fatalf("unexpected prerelease increment %s", got.String())
	}
	if got := base.Bump(BumpPatch, "beta.{n}").WithPrerelease("beta.{n}"); got.String() != "1.2.4-beta.1" {
		t.Fatalf("unexpected patch prerelease bump %s", got.String())
	}
}

func TestGenerateChangelog(t *testing.T) {
	changelog := GenerateChangelog([]Commit{
		{Hash: "abcdef123456", Subject: "feat(ui): add dashboard"},
		{Hash: "123456abcdef", Subject: "fix: handle missing config"},
	}, Version{Major: 1, Minor: 2, Patch: 3}, Version{Major: 1, Minor: 3, Patch: 0})

	if !strings.Contains(changelog, "Release 1.3.0") {
		t.Fatalf("expected release heading, got %q", changelog)
	}
	if !strings.Contains(changelog, "### Features") {
		t.Fatalf("expected features section, got %q", changelog)
	}
	if !strings.Contains(changelog, "add dashboard") {
		t.Fatalf("expected cleaned conventional subject, got %q", changelog)
	}
}

func TestGitManagerLatestCommitsAndDirty(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repo := newGitRepo(t)
	writeRepoFile(t, repo, "README.md", "hello\n")
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-m", "feat: initial release")
	gitRun(t, repo, "tag", "v1.2.3")
	writeRepoFile(t, repo, "README.md", "hello\nworld\n")
	gitRun(t, repo, "commit", "-am", "fix: add second line")

	manager := NewGitManager(repo, "v")

	tag, version, err := manager.Latest(context.Background())
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if tag != "v1.2.3" {
		t.Fatalf("unexpected tag %q", tag)
	}
	if version.String() != "1.2.3" {
		t.Fatalf("unexpected version %s", version.String())
	}

	commits, err := manager.CommitsSince(context.Background(), "v1.2.3")
	if err != nil {
		t.Fatalf("commits since: %v", err)
	}
	if len(commits) != 1 || commits[0].Type != "fix" {
		t.Fatalf("unexpected commits %+v", commits)
	}

	writeRepoFile(t, repo, "dirty.txt", "pending\n")
	dirty, err := manager.IsDirty(context.Background())
	if err != nil {
		t.Fatalf("is dirty: %v", err)
	}
	if !dirty {
		t.Fatal("expected dirty repo")
	}
}

func TestGitManagerTagPush(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	remote := filepath.Join(t.TempDir(), "remote.git")
	gitRun(t, "", "init", "--bare", remote)

	repo := newGitRepo(t)
	writeRepoFile(t, repo, "README.md", "hello\n")
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-m", "feat: initial release")
	gitRun(t, repo, "remote", "add", "origin", remote)

	manager := NewGitManager(repo, "v")
	if err := manager.Tag(context.Background(), Version{Major: 1, Minor: 3, Patch: 0}, "Release 1.3.0", true); err != nil {
		t.Fatalf("tag push: %v", err)
	}

	output := gitRun(t, "", "--git-dir", remote, "tag", "--list")
	if !strings.Contains(output, "v1.3.0") {
		t.Fatalf("expected remote tag, got %q", output)
	}
}

func gotPrerelease(base Version) Version {
	return base.Bump(BumpPrerelease, "beta.{n}")
}

func newGitRepo(t *testing.T) string {
	t.Helper()

	repo := t.TempDir()
	gitRun(t, repo, "init")
	gitRun(t, repo, "config", "user.name", "Test User")
	gitRun(t, repo, "config", "user.email", "test@example.com")
	return repo
}

func writeRepoFile(t *testing.T, dir, name, content string) {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
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
