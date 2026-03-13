package wailsupdate

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Eriyc/wailsrel/pkg/update"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type stubChecker struct {
	check func(context.Context, update.CheckOpts) (*update.CheckResult, error)
}

func (s stubChecker) Check(ctx context.Context, opts update.CheckOpts) (*update.CheckResult, error) {
	return s.check(ctx, opts)
}

type stubApplier struct {
	applyNative func(context.Context, *update.UpdateInfo, update.ProgressFunc) error
}

func (s stubApplier) ApplyNative(ctx context.Context, info *update.UpdateInfo, progress update.ProgressFunc) error {
	return s.applyNative(ctx, info, progress)
}

func (s stubApplier) ApplyFrontend(ctx context.Context, info *update.FrontendUpdateInfo, progress update.ProgressFunc) error {
	return nil
}

func TestNewServiceDefaults(t *testing.T) {
	service := NewService(Options{})

	if service.opts.EventPrefix != defaultEventPrefix {
		t.Fatalf("expected default event prefix %q, got %q", defaultEventPrefix, service.opts.EventPrefix)
	}
	if service.opts.CheckTimeout != defaultCheckTimeout {
		t.Fatalf("expected default check timeout %s, got %s", defaultCheckTimeout, service.opts.CheckTimeout)
	}
	if service.opts.ApplyTimeout != defaultApplyTimeout {
		t.Fatalf("expected default apply timeout %s, got %s", defaultApplyTimeout, service.opts.ApplyTimeout)
	}
	if service.opts.TargetPath == "" {
		t.Fatal("expected default target path to be populated")
	}
	if service.opts.TempDir == "" {
		t.Fatal("expected temp dir to be populated")
	}
	if service.opts.Checker == nil {
		t.Fatal("expected default checker")
	}
	if service.opts.Applier == nil {
		t.Fatal("expected default applier")
	}
}

func TestGetStateIncludesHashAndDecorator(t *testing.T) {
	targetPath := writeTempFile(t, "binary-data")

	service := NewService(Options{
		ManifestURL:    "https://example.com/manifest.json",
		CurrentVersion: "1.0.0",
		TargetPath:     targetPath,
		DescribeState: func(state *State) {
			if state.Metadata == nil {
				state.Metadata = map[string]string{}
			}
			state.Metadata["repository"] = "owner/repo"
			state.Notes = append(state.Notes, "decorated")
		},
	})

	state := service.GetState()
	if state.CurrentHash == "" {
		t.Fatal("expected current hash")
	}
	if state.Metadata["repository"] != "owner/repo" {
		t.Fatalf("expected decorator metadata, got %#v", state.Metadata)
	}
	if len(state.Notes) == 0 || state.Notes[len(state.Notes)-1] != "decorated" {
		t.Fatalf("expected decorator note, got %#v", state.Notes)
	}
}

func TestCheckNowValidatesRequiredFields(t *testing.T) {
	service := NewService(Options{
		CurrentVersion: "1.0.0",
		TargetPath:     writeTempFile(t, "binary-data"),
	})

	response := service.CheckNow()
	if response.Error == "" {
		t.Fatal("expected validation error")
	}
	if response.Error != "manifest URL is required" {
		t.Fatalf("unexpected error: %s", response.Error)
	}
}

func TestCheckNowCachesAndClearsAvailableState(t *testing.T) {
	targetPath := writeTempFile(t, "binary-data")
	var calls int32

	service := NewService(Options{
		ManifestURL:    "https://example.com/manifest.json",
		CurrentVersion: "1.0.0",
		TargetPath:     targetPath,
		Checker: stubChecker{check: func(ctx context.Context, opts update.CheckOpts) (*update.CheckResult, error) {
			if atomic.AddInt32(&calls, 1) == 1 {
				return &update.CheckResult{
					Available: true,
					Native: &update.UpdateInfo{
						Version:      "1.1.0",
						Channel:      "stable",
						ArtifactURL:  "https://example.com/app.bin",
						ArtifactHash: "sha256:next",
					},
				}, nil
			}
			return &update.CheckResult{}, nil
		}},
		Applier: stubApplier{applyNative: func(context.Context, *update.UpdateInfo, update.ProgressFunc) error { return nil }},
	})

	first := service.CheckNow()
	if !first.Available || first.Update == nil {
		t.Fatalf("expected cached update, got %+v", first)
	}
	if state := service.GetState(); state.AvailableUpdate == nil {
		t.Fatal("expected state to include cached update")
	}

	second := service.CheckNow()
	if second.Available {
		t.Fatalf("expected no available update, got %+v", second)
	}
	if state := service.GetState(); state.AvailableUpdate != nil {
		t.Fatalf("expected cached update to be cleared, got %+v", state.AvailableUpdate)
	}
}

func TestApplyPendingStagesWithoutRestart(t *testing.T) {
	targetPath := writeTempFile(t, "binary-data")
	updateInfo := &update.UpdateInfo{
		Version:      "1.1.0",
		Channel:      "stable",
		ArtifactURL:  "https://example.com/app.bin",
		ArtifactHash: "sha256:next",
	}

	var relaunchCalls int32
	service := NewService(Options{
		ManifestURL:    "https://example.com/manifest.json",
		CurrentVersion: "1.0.0",
		TargetPath:     targetPath,
		Checker: stubChecker{check: func(ctx context.Context, opts update.CheckOpts) (*update.CheckResult, error) {
			return &update.CheckResult{Available: true, Native: updateInfo}, nil
		}},
		Applier: stubApplier{applyNative: func(ctx context.Context, info *update.UpdateInfo, progress update.ProgressFunc) error {
			if !reflect.DeepEqual(info, updateInfo) {
				t.Fatalf("unexpected update info: %+v", info)
			}
			progress(5, 10)
			return nil
		}},
		Relaunch: func(context.Context, RelaunchRequest) error {
			atomic.AddInt32(&relaunchCalls, 1)
			return nil
		},
	})

	response := service.ApplyPending()
	if !response.Applied {
		t.Fatalf("expected apply success, got %+v", response)
	}
	if response.Restarted {
		t.Fatalf("apply should not restart: %+v", response)
	}
	if atomic.LoadInt32(&relaunchCalls) != 0 {
		t.Fatal("relaunch should not be called during apply")
	}
	if state := service.GetState(); !state.PendingRestart {
		t.Fatalf("expected pending restart state, got %+v", state)
	}
}

func TestApplyPendingChecksWhenNoCachedUpdate(t *testing.T) {
	targetPath := writeTempFile(t, "binary-data")
	var checks int32
	var applies int32

	service := NewService(Options{
		ManifestURL:    "https://example.com/manifest.json",
		CurrentVersion: "1.0.0",
		TargetPath:     targetPath,
		Checker: stubChecker{check: func(ctx context.Context, opts update.CheckOpts) (*update.CheckResult, error) {
			atomic.AddInt32(&checks, 1)
			return &update.CheckResult{
				Available: true,
				Native: &update.UpdateInfo{
					Version:      "1.1.0",
					Channel:      "stable",
					ArtifactURL:  "https://example.com/app.bin",
					ArtifactHash: "sha256:next",
				},
			}, nil
		}},
		Applier: stubApplier{applyNative: func(ctx context.Context, info *update.UpdateInfo, progress update.ProgressFunc) error {
			atomic.AddInt32(&applies, 1)
			return nil
		}},
	})

	response := service.ApplyPending()
	if !response.Applied {
		t.Fatalf("expected apply success, got %+v", response)
	}
	if atomic.LoadInt32(&checks) != 1 {
		t.Fatalf("expected one check, got %d", checks)
	}
	if atomic.LoadInt32(&applies) != 1 {
		t.Fatalf("expected one apply, got %d", applies)
	}
}

func TestRestartNoopWithoutPendingStage(t *testing.T) {
	service := NewService(Options{})
	quitCount := withQuitStub(func() {
		response := service.Restart()
		if response.Restarted {
			t.Fatalf("expected no-op restart, got %+v", response)
		}
	})
	if quitCount != 0 {
		t.Fatalf("expected no quit, got %d", quitCount)
	}
}

func TestRestartInvokesRelaunchAndQuitsOnSuccess(t *testing.T) {
	targetPath := writeTempFile(t, "binary-data")
	service := NewService(Options{
		TargetPath: targetPath,
		Relaunch: func(ctx context.Context, request RelaunchRequest) error {
			if request.Executable != DefaultTargetPath() {
				t.Fatalf("unexpected executable: %s", request.Executable)
			}
			if !reflect.DeepEqual(request.Args, os.Args[1:]) {
				t.Fatalf("unexpected args: %#v", request.Args)
			}
			return nil
		},
	})
	service.markPendingRestart()

	quitCount := withQuitStub(func() {
		response := service.Restart()
		if !response.Restarted {
			t.Fatalf("expected restart success, got %+v", response)
		}
		if service.GetState().PendingRestart {
			t.Fatal("expected pending restart to be cleared")
		}
	})
	if quitCount != 1 {
		t.Fatalf("expected one quit, got %d", quitCount)
	}
}

func TestRestartPropagatesRelaunchFailure(t *testing.T) {
	targetPath := writeTempFile(t, "binary-data")
	service := NewService(Options{
		TargetPath: targetPath,
		Relaunch: func(ctx context.Context, request RelaunchRequest) error {
			return io.EOF
		},
	})
	service.markPendingRestart()

	quitCount := withQuitStub(func() {
		response := service.Restart()
		if response.Error == "" {
			t.Fatalf("expected relaunch error, got %+v", response)
		}
		if !service.GetState().PendingRestart {
			t.Fatal("expected pending restart to remain set")
		}
	})
	if quitCount != 0 {
		t.Fatalf("expected no quit on relaunch failure, got %d", quitCount)
	}
}

func TestAutoCheckRunsAndStopsOnShutdown(t *testing.T) {
	targetPath := writeTempFile(t, "binary-data")
	var checks int32

	service := NewService(Options{
		ManifestURL:       "https://example.com/manifest.json",
		CurrentVersion:    "1.0.0",
		TargetPath:        targetPath,
		AutoCheckInterval: 10 * time.Millisecond,
		Checker: stubChecker{check: func(ctx context.Context, opts update.CheckOpts) (*update.CheckResult, error) {
			atomic.AddInt32(&checks, 1)
			return &update.CheckResult{}, nil
		}},
		Applier: stubApplier{applyNative: func(ctx context.Context, info *update.UpdateInfo, progress update.ProgressFunc) error { return nil }},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := service.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatalf("startup failed: %v", err)
	}

	deadline := time.Now().Add(300 * time.Millisecond)
	for atomic.LoadInt32(&checks) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt32(&checks) == 0 {
		t.Fatal("expected auto-check to run")
	}

	if err := service.ServiceShutdown(); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}
	countAfterShutdown := atomic.LoadInt32(&checks)
	time.Sleep(40 * time.Millisecond)
	if atomic.LoadInt32(&checks) != countAfterShutdown {
		t.Fatal("expected auto-check to stop after shutdown")
	}
}

func TestBearerTransportAddsAuthorizationWithoutMutatingRequest(t *testing.T) {
	var gotAuth string
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotAuth = req.Header.Get("Authorization")
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})

	request, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	request.Header.Set("X-Test", "value")

	response, err := BearerTransport(base, "token-123").RoundTrip(request)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	_ = response.Body.Close()

	if gotAuth != "Bearer token-123" {
		t.Fatalf("unexpected auth header: %q", gotAuth)
	}
	if request.Header.Get("Authorization") != "" {
		t.Fatal("expected original request to remain unchanged")
	}
}

func TestGitHubLatestManifestURL(t *testing.T) {
	got := GitHubLatestManifestURL("owner/repo")
	want := "https://github.com/owner/repo/releases/latest/download/manifest.json"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func writeTempFile(t *testing.T, contents string) string {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "app.bin")
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

func withQuitStub(run func()) int {
	var count int32
	previous := quitApplication
	quitApplication = func() {
		atomic.AddInt32(&count, 1)
	}
	defer func() {
		quitApplication = previous
	}()
	run()
	return int(atomic.LoadInt32(&count))
}
