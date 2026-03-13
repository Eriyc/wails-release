package build

import "github.com/Eriyc/wailsrel/pkg/config"

func ExpandMatrix(targets []config.TargetConfig) []Target {
	matrix := make([]Target, 0, len(targets))
	for _, target := range targets {
		for _, arch := range target.Arch {
			matrix = append(matrix, Target{
				OS:            target.OS,
				Arch:          arch,
				OutputFormats: append([]string(nil), target.OutputFormats...),
				Sign:          target.Sign,
			})
		}
	}

	return matrix
}
