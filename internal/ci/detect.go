package ci

import "os"

type Info struct {
	IsCI            bool   `json:"is_ci"`
	Provider        string `json:"provider"`
	IsGitHubActions bool   `json:"is_github_actions"`
	Workspace       string `json:"workspace,omitempty"`
	OutputFile      string `json:"output_file,omitempty"`
}

func Detect() Info {
	info := Info{
		IsCI: os.Getenv("CI") != "",
	}

	if os.Getenv("GITHUB_ACTIONS") == "true" {
		info.IsCI = true
		info.Provider = "github"
		info.IsGitHubActions = true
		info.Workspace = os.Getenv("GITHUB_WORKSPACE")
		info.OutputFile = os.Getenv("GITHUB_OUTPUT")
	}

	return info
}
