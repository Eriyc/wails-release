package version

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

var ErrNoVersionTags = errors.New("no semantic version tags found")

type GitManager interface {
	CurrentVersion(ctx context.Context) (Version, error)
	Tag(ctx context.Context, v Version, message string, push bool) error
	IsDirty(ctx context.Context) (bool, error)
	CommitsSince(ctx context.Context, tag string) ([]Commit, error)
}

type Commit struct {
	Hash    string
	Subject string
	Body    string
	Type    string
}

type Git struct {
	Dir       string
	TagPrefix string
}

func NewGitManager(dir, tagPrefix string) *Git {
	return &Git{
		Dir:       dir,
		TagPrefix: tagPrefix,
	}
}

func (g *Git) CurrentVersion(ctx context.Context) (Version, error) {
	_, version, err := g.Latest(ctx)
	return version, err
}

func (g *Git) Latest(ctx context.Context) (string, Version, error) {
	output, err := g.run(ctx, "tag", "--list")
	if err != nil {
		return "", Version{}, err
	}

	var (
		latestTag     string
		latestVersion Version
		found         bool
	)

	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		tag := strings.TrimSpace(line)
		if tag == "" {
			continue
		}
		if g.TagPrefix != "" && !strings.HasPrefix(tag, g.TagPrefix) {
			continue
		}

		raw := strings.TrimPrefix(tag, g.TagPrefix)
		parsed, err := Parse(raw)
		if err != nil {
			continue
		}

		if !found || parsed.Compare(latestVersion) > 0 {
			found = true
			latestTag = tag
			latestVersion = parsed
		}
	}

	if !found {
		return "", Version{}, ErrNoVersionTags
	}

	return latestTag, latestVersion, nil
}

func (g *Git) Tag(ctx context.Context, v Version, message string, push bool) error {
	tagName := g.TagPrefix + v.String()
	if strings.TrimSpace(message) == "" {
		message = "Release " + tagName
	}

	if _, err := g.run(ctx, "tag", "-a", tagName, "-m", message); err != nil {
		return err
	}

	if push {
		if _, err := g.run(ctx, "push", "origin", tagName); err != nil {
			return err
		}
	}

	return nil
}

func (g *Git) IsDirty(ctx context.Context) (bool, error) {
	output, err := g.run(ctx, "status", "--porcelain")
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(output) != "", nil
}

func (g *Git) CommitsSince(ctx context.Context, tag string) ([]Commit, error) {
	args := []string{"log", "--format=%H%x1f%s%x1f%b%x1e"}
	if strings.TrimSpace(tag) != "" {
		args = append(args, tag+"..HEAD")
	}

	output, err := g.run(ctx, args...)
	if err != nil {
		return nil, err
	}

	records := strings.Split(output, "\x1e")
	commits := make([]Commit, 0, len(records))
	for _, record := range records {
		record = strings.TrimSpace(record)
		if record == "" {
			continue
		}

		fields := strings.Split(record, "\x1f")
		if len(fields) < 3 {
			continue
		}

		commit := Commit{
			Hash:    strings.TrimSpace(fields[0]),
			Subject: strings.TrimSpace(fields[1]),
			Body:    strings.TrimSpace(fields[2]),
		}
		commit.Type = conventionalType(commit.Subject)
		commits = append(commits, commit)
	}

	return commits, nil
}

func (g *Git) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = g.workDir()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errText := strings.TrimSpace(stderr.String())
		if errText == "" {
			errText = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), errText)
	}

	return stdout.String(), nil
}

func (g *Git) workDir() string {
	if g.Dir == "" {
		return "."
	}
	return filepath.Clean(g.Dir)
}
