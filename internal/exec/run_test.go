package exec

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestRunCapturesOutput(t *testing.T) {
	result, err := Run(context.Background(), os.Args[0], helperArgs("echo"), Options{})
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
	_, err := Run(context.Background(), os.Args[0], helperArgs("sleep"), Options{Timeout: 100 * time.Millisecond})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func helperArgs(mode string) []string {
	return []string{"-test.run=TestHelperProcess", "--", mode}
}

func TestHelperProcess(t *testing.T) {
	if len(os.Args) < 3 || os.Args[1] != "--" {
		return
	}

	switch os.Args[2] {
	case "echo":
		fmt.Fprint(os.Stdout, "stdout line\n")
		fmt.Fprint(os.Stderr, "stderr line\n")
	case "sleep":
		time.Sleep(2 * time.Second)
	default:
		t.Fatalf("unknown helper mode %q", os.Args[2])
	}

	os.Exit(0)
}
