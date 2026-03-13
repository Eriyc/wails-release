package build

import "github.com/Eriyc/wailsrel/pkg/config"

func ExpandMatrix(targets []config.TargetConfig) []Target {
	matrix := make([]Target, 0, len(targets))
	for _, target := range targets {
		matrix = append(matrix, Target{
			ID:        target.ID,
			OS:        target.OS,
			Arch:      target.Arch,
			Build:     target.Build,
			Artifacts: append([]config.ArtifactSpec(nil), target.Artifacts...),
		})
	}

	return matrix
}
