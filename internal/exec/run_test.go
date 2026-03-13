package exec

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestRunCapturesOutput(t *testing.T) {
	result, err := Run(context.Background(), os.Args[0], helperArgs(), Options{
		Env: []string{
			"GO_WANT_HELPER_PROCESS=1",
			"HELPER_MODE=echo",
		},
	})
	if err != nil {
		t.Fatalf("run helper: %v", err)
	}

	if result.Stdout != "stdout line\n" {
		t.Fatalf("unexpected stdout %q", result.Stdout)
	}
	if result.Stderr != "stderr line\n" {
		t.Fatalf("unexpected stderr %q", result.Stderr)
	}
	if result.ExitCode != 0 {
		t.Fatalf("unexpected exit code %d", result.ExitCode)
	}
}

func TestRunTimeout(t *testing.T) {
	_, err := Run(context.Background(), os.Args[0], helperArgs(), Options{
		Env: []string{
			"GO_WANT_HELPER_PROCESS=1",
			"HELPER_MODE=sleep",
		},
		Timeout: 100 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func helperArgs() []string {
	return []string{"-test.run=TestHelperProcess"}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	switch os.Getenv("HELPER_MODE") {
	case "echo":
		fmt.Fprint(os.Stdout, "stdout line\n")
		fmt.Fprint(os.Stderr, "stderr line\n")
	case "sleep":
		time.Sleep(2 * time.Second)
	default:
		t.Fatalf("unknown helper mode %q", os.Getenv("HELPER_MODE"))
	}

	os.Exit(0)
}
