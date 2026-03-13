package version

import (
	"fmt"
	"strings"
)

func GenerateChangelog(commits []Commit, from, to Version) string {
	var builder strings.Builder

	fmt.Fprintf(&builder, "Release %s\n", to.String())
	if from != (Version{}) {
		fmt.Fprintf(&builder, "\nChanges from %s to %s.\n", from.String(), to.String())
	}

	breaking := filterCommits(commits, isBreakingCommit)
	features := filterCommits(commits, func(commit Commit) bool { return commitType(commit) == "feat" && !isBreakingCommit(commit) })
	fixes := filterCommits(commits, func(commit Commit) bool { return commitType(commit) == "fix" })
	maintenance := filterCommits(commits, func(commit Commit) bool {
		switch commitType(commit) {
		case "chore", "refactor", "perf", "build", "ci", "docs", "test", "style", "revert":
			return true
		default:
			return false
		}
	})
	other := filterCommits(commits, func(commit Commit) bool {
		switch commitType(commit) {
		case "", "other":
			return true
		default:
			return false
		}
	})

	writeCommitSection(&builder, "Breaking Changes", breaking)
	writeCommitSection(&builder, "Features", features)
	writeCommitSection(&builder, "Fixes", fixes)
	writeCommitSection(&builder, "Maintenance", maintenance)
	writeCommitSection(&builder, "Other Changes", other)

	return strings.TrimSpace(builder.String())
}

func writeCommitSection(builder *strings.Builder, title string, commits []Commit) {
	if len(commits) == 0 {
		return
	}

	fmt.Fprintf(builder, "\n### %s\n", title)
	for _, commit := range commits {
		fmt.Fprintf(builder, "- %s (%s)\n", cleanCommitSubject(commit.Subject), shortHash(commit.Hash))
	}
}

func filterCommits(commits []Commit, keep func(Commit) bool) []Commit {
	out := make([]Commit, 0, len(commits))
	for _, commit := range commits {
		if keep(commit) {
			out = append(out, commit)
		}
	}
	return out
}

func commitType(commit Commit) string {
	if commit.Type != "" {
		return commit.Type
	}
	return conventionalType(commit.Subject)
}

func shortHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}

func cleanCommitSubject(subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return "update"
	}

	if idx := strings.Index(subject, ":"); idx > 0 {
		prefix := subject[:idx]
		if isConventionalPrefix(prefix) {
			return strings.TrimSpace(subject[idx+1:])
		}
	}

	return subject
}

func conventionalType(subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return ""
	}

	idx := strings.Index(subject, ":")
	if idx < 0 {
		return "other"
	}

	prefix := subject[:idx]
	base := prefix
	if scopeStart := strings.Index(prefix, "("); scopeStart >= 0 {
		base = prefix[:scopeStart]
	}
	base = strings.TrimSuffix(base, "!")
	if base == "" || !isConventionalPrefix(prefix) {
		return "other"
	}

	return base
}

func isConventionalPrefix(prefix string) bool {
	base := prefix
	if scopeStart := strings.Index(prefix, "("); scopeStart >= 0 {
		base = prefix[:scopeStart]
	}
	base = strings.TrimSuffix(base, "!")

	switch base {
	case "feat", "fix", "chore", "docs", "refactor", "perf", "test", "build", "ci", "style", "revert":
		return true
	default:
		return false
	}
}

func isBreakingCommit(commit Commit) bool {
	subjectPrefix := commit.Subject
	if idx := strings.Index(subjectPrefix, ":"); idx >= 0 {
		subjectPrefix = subjectPrefix[:idx]
	}

	return strings.HasSuffix(subjectPrefix, "!") || strings.Contains(commit.Body, "BREAKING CHANGE:")
}
