package ci

import "testing"

func TestDetectLocal(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("GITHUB_WORKSPACE", "")
	t.Setenv("GITHUB_OUTPUT", "")

	info := Detect()
	if info.IsCI {
		t.Fatal("expected local detection")
	}
	if info.Provider != "" {
		t.Fatalf("expected empty provider, got %q", info.Provider)
	}
}

func TestDetectGitHubActions(t *testing.T) {
	t.Setenv("CI", "true")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_WORKSPACE", "/tmp/workspace")
	t.Setenv("GITHUB_OUTPUT", "/tmp/output")

	info := Detect()
	if !info.IsCI || !info.IsGitHubActions {
		t.Fatal("expected GitHub Actions detection")
	}
	if info.Provider != "github" {
		t.Fatalf("expected github provider, got %q", info.Provider)
	}
	if info.Workspace != "/tmp/workspace" {
		t.Fatalf("unexpected workspace %q", info.Workspace)
	}
}
