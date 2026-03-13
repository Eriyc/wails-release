package version

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var semverPattern = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)

type Version struct {
	Major      int
	Minor      int
	Patch      int
	Prerelease string
	Metadata   string
}

func Parse(s string) (Version, error) {
	matches := semverPattern.FindStringSubmatch(strings.TrimSpace(s))
	if matches == nil {
		return Version{}, fmt.Errorf("invalid semantic version %q", s)
	}

	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])
	patch, _ := strconv.Atoi(matches[3])

	v := Version{
		Major:      major,
		Minor:      minor,
		Patch:      patch,
		Prerelease: matches[4],
		Metadata:   matches[5],
	}

	if err := validatePrerelease(v.Prerelease); err != nil {
		return Version{}, err
	}

	return v, nil
}

func (v Version) String() string {
	value := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Prerelease != "" {
		value += "-" + v.Prerelease
	}
	if v.Metadata != "" {
		value += "+" + v.Metadata
	}
	return value
}

func (v Version) Compare(other Version) int {
	if cmp := compareInt(v.Major, other.Major); cmp != 0 {
		return cmp
	}
	if cmp := compareInt(v.Minor, other.Minor); cmp != 0 {
		return cmp
	}
	if cmp := compareInt(v.Patch, other.Patch); cmp != 0 {
		return cmp
	}

	return comparePrerelease(v.Prerelease, other.Prerelease)
}

func (v Version) IsPrerelease() bool {
	return v.Prerelease != ""
}

func (v Version) WithPrerelease(format string) Version {
	next := v
	next.Metadata = ""
	next.Prerelease = renderPrerelease(next.Prerelease, format)
	return next
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func comparePrerelease(a, b string) int {
	switch {
	case a == "" && b == "":
		return 0
	case a == "":
		return 1
	case b == "":
		return -1
	}

	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")
	limit := min(len(aParts), len(bParts))
	for i := range limit {
		aPart := aParts[i]
		bPart := bParts[i]

		aNum, aNumeric := numericIdentifier(aPart)
		bNum, bNumeric := numericIdentifier(bPart)

		switch {
		case aNumeric && bNumeric:
			if cmp := compareInt(aNum, bNum); cmp != 0 {
				return cmp
			}
		case aNumeric:
			return -1
		case bNumeric:
			return 1
		default:
			if aPart < bPart {
				return -1
			}
			if aPart > bPart {
				return 1
			}
		}
	}

	return compareInt(len(aParts), len(bParts))
}

func validatePrerelease(value string) error {
	if value == "" {
		return nil
	}

	for _, part := range strings.Split(value, ".") {
		if part == "" {
			return fmt.Errorf("invalid prerelease %q", value)
		}
		if number, numeric := numericIdentifier(part); numeric && strconv.Itoa(number) != part {
			return fmt.Errorf("invalid prerelease %q: numeric identifiers must not contain leading zeroes", value)
		}
	}

	return nil
}

func numericIdentifier(value string) (int, bool) {
	if value == "" {
		return 0, false
	}

	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, false
		}
	}

	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}

	return n, true
}
