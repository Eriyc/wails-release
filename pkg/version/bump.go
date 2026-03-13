package version

import (
	"fmt"
	"strconv"
	"strings"
)

type BumpType int

const (
	BumpPatch BumpType = iota
	BumpMinor
	BumpMajor
	BumpPrerelease
)

func (v Version) Bump(t BumpType, preFormat string) Version {
	next := v
	next.Metadata = ""

	switch t {
	case BumpPatch:
		next.Patch++
		next.Prerelease = ""
	case BumpMinor:
		next.Minor++
		next.Patch = 0
		next.Prerelease = ""
	case BumpMajor:
		next.Major++
		next.Minor = 0
		next.Patch = 0
		next.Prerelease = ""
	case BumpPrerelease:
		next.Prerelease = renderPrerelease(next.Prerelease, preFormat)
	default:
		panic(fmt.Sprintf("unsupported bump type %d", t))
	}

	return next
}

func renderPrerelease(current, format string) string {
	format = strings.TrimSpace(format)
	if format == "" {
		format = "beta.{n}"
	}

	if strings.Contains(format, "{n}") {
		n := 1
		if parsed, ok := extractFormatNumber(current, format); ok {
			n = parsed + 1
		}
		return strings.ReplaceAll(format, "{n}", strconv.Itoa(n))
	}

	if current == format {
		return format + ".2"
	}

	if parsed, ok := incrementNumericSuffix(current); ok {
		return parsed
	}

	return format
}

func extractFormatNumber(current, format string) (int, bool) {
	parts := strings.SplitN(format, "{n}", 2)
	prefix := parts[0]
	suffix := parts[1]

	if !strings.HasPrefix(current, prefix) || !strings.HasSuffix(current, suffix) {
		return 0, false
	}

	numberText := strings.TrimSuffix(strings.TrimPrefix(current, prefix), suffix)
	if numberText == "" {
		return 0, false
	}

	number, ok := numericIdentifier(numberText)
	if !ok {
		return 0, false
	}

	return number, true
}

func incrementNumericSuffix(current string) (string, bool) {
	if current == "" {
		return "", false
	}

	parts := strings.Split(current, ".")
	last, ok := numericIdentifier(parts[len(parts)-1])
	if !ok {
		return "", false
	}

	parts[len(parts)-1] = strconv.Itoa(last + 1)
	return strings.Join(parts, "."), true
}
