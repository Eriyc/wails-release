package update

import (
	"github.com/Eriyc/wailsrel/gen/go/wailsrel/v1"
)

func CheckOptsToProto(opts CheckOpts) *wailsrelv1.UpdateCheckOptions {
	return &wailsrelv1.UpdateCheckOptions{
		CurrentVersion: opts.CurrentVersion,
		CurrentHash:    opts.CurrentHash,
		NativeCompat:   opts.NativeCompat,
		Channel:        opts.Channel,
		ManifestUrl:    opts.ManifestURL,
	}
}

func CheckOptsFromProto(message *wailsrelv1.UpdateCheckOptions) CheckOpts {
	if message == nil {
		return CheckOpts{}
	}
	return CheckOpts{
		CurrentVersion: message.CurrentVersion,
		CurrentHash:    message.CurrentHash,
		NativeCompat:   message.NativeCompat,
		Channel:        message.Channel,
		ManifestURL:    message.ManifestUrl,
	}
}

func FrontendUpdateInfoToProto(info *FrontendUpdateInfo) *wailsrelv1.UpdateFrontendInfo {
	if info == nil {
		return nil
	}
	return &wailsrelv1.UpdateFrontendInfo{
		Channel:  info.Channel,
		Version:  info.Version,
		CompatId: info.CompatID,
		Url:      info.URL,
		Hash:     info.Hash,
		Size:     info.Size,
	}
}

func FrontendUpdateInfoFromProto(message *wailsrelv1.UpdateFrontendInfo) *FrontendUpdateInfo {
	if message == nil {
		return nil
	}
	return &FrontendUpdateInfo{
		Channel:  message.Channel,
		Version:  message.Version,
		CompatID: message.CompatId,
		URL:      message.Url,
		Hash:     message.Hash,
		Size:     message.Size,
	}
}

func UpdateInfoToProto(info *UpdateInfo) *wailsrelv1.UpdateInfo {
	if info == nil {
		return nil
	}
	return &wailsrelv1.UpdateInfo{
		Version:       info.Version,
		Channel:       info.Channel,
		ReleaseNotes:  info.ReleaseNotes,
		Mandatory:     info.Mandatory,
		ArtifactUrl:   info.ArtifactURL,
		ArtifactHash:  info.ArtifactHash,
		ArtifactSize:  info.ArtifactSize,
		DeltaUrl:      info.DeltaURL,
		DeltaHash:     info.DeltaHash,
		DeltaSize:     info.DeltaSize,
		DeltaFromHash: info.DeltaFromHash,
		Frontend:      FrontendUpdateInfoToProto(info.Frontend),
	}
}

func UpdateInfoFromProto(message *wailsrelv1.UpdateInfo) *UpdateInfo {
	if message == nil {
		return nil
	}
	return &UpdateInfo{
		Version:       message.Version,
		Channel:       message.Channel,
		ReleaseNotes:  message.ReleaseNotes,
		Mandatory:     message.Mandatory,
		ArtifactURL:   message.ArtifactUrl,
		ArtifactHash:  message.ArtifactHash,
		ArtifactSize:  message.ArtifactSize,
		DeltaURL:      message.DeltaUrl,
		DeltaHash:     message.DeltaHash,
		DeltaSize:     message.DeltaSize,
		DeltaFromHash: message.DeltaFromHash,
		Frontend:      FrontendUpdateInfoFromProto(message.Frontend),
	}
}

func CheckResultToProto(result *CheckResult) *wailsrelv1.UpdateCheckResult {
	if result == nil {
		return nil
	}
	return &wailsrelv1.UpdateCheckResult{
		Available: result.Available,
		Native:    UpdateInfoToProto(result.Native),
		Frontend:  FrontendUpdateInfoToProto(result.Frontend),
	}
}

func CheckResultFromProto(message *wailsrelv1.UpdateCheckResult) *CheckResult {
	if message == nil {
		return nil
	}
	return &CheckResult{
		Available: message.Available,
		Native:    UpdateInfoFromProto(message.Native),
		Frontend:  FrontendUpdateInfoFromProto(message.Frontend),
	}
}
