package wailsupdate

import (
	"time"

	"github.com/Eriyc/wailsrel/gen/go/wailsrel/v1"
	"github.com/Eriyc/wailsrel/pkg/contract"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func UpdateViewToProto(view *UpdateView) *wailsrelv1.UpdaterUpdateView {
	if view == nil {
		return nil
	}
	return &wailsrelv1.UpdaterUpdateView{
		Version:       view.Version,
		Channel:       view.Channel,
		ReleaseNotes:  view.ReleaseNotes,
		Mandatory:     view.Mandatory,
		ArtifactUrl:   view.ArtifactURL,
		ArtifactHash:  view.ArtifactHash,
		ArtifactSize:  view.ArtifactSize,
		DeltaUrl:      view.DeltaURL,
		DeltaHash:     view.DeltaHash,
		DeltaSize:     view.DeltaSize,
		DeltaFromHash: view.DeltaFromHash,
		FrontendUrl:   view.FrontendURL,
		FrontendHash:  view.FrontendHash,
		FrontendSize:  view.FrontendSize,
		FrontendOnly:  view.FrontendOnly,
	}
}

func UpdateViewFromProto(message *wailsrelv1.UpdaterUpdateView) *UpdateView {
	if message == nil {
		return nil
	}
	return &UpdateView{
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
		FrontendURL:   message.FrontendUrl,
		FrontendHash:  message.FrontendHash,
		FrontendSize:  message.FrontendSize,
		FrontendOnly:  message.FrontendOnly,
	}
}

func StateToProto(state State) *wailsrelv1.UpdaterState {
	return &wailsrelv1.UpdaterState{
		ManifestUrl:     state.ManifestURL,
		CurrentVersion:  state.CurrentVersion,
		CurrentHash:     state.CurrentHash,
		Channel:         state.Channel,
		NativeCompat:    state.NativeCompat,
		TargetPath:      state.TargetPath,
		TempDir:         state.TempDir,
		LastCheckedAt:   parseUpdaterTimestamp(state.LastCheckedAt),
		PendingRestart:  state.PendingRestart,
		AvailableUpdate: UpdateViewToProto(state.AvailableUpdate),
		Metadata:        cloneUpdaterMetadata(state.Metadata),
		Notes:           append([]string(nil), state.Notes...),
		LastError:       state.LastError,
	}
}

func StateFromProto(message *wailsrelv1.UpdaterState) State {
	if message == nil {
		return State{}
	}
	return State{
		ManifestURL:     message.ManifestUrl,
		CurrentVersion:  message.CurrentVersion,
		CurrentHash:     message.CurrentHash,
		Channel:         message.Channel,
		NativeCompat:    message.NativeCompat,
		TargetPath:      message.TargetPath,
		TempDir:         message.TempDir,
		LastCheckedAt:   formatUpdaterTimestamp(message.LastCheckedAt),
		PendingRestart:  message.PendingRestart,
		AvailableUpdate: UpdateViewFromProto(message.AvailableUpdate),
		Metadata:        cloneUpdaterMetadata(message.Metadata),
		Notes:           append([]string(nil), message.Notes...),
		LastError:       message.LastError,
	}
}

func CheckResponseToProto(response CheckResponse) *wailsrelv1.UpdaterCheckResponse {
	return &wailsrelv1.UpdaterCheckResponse{
		CheckedAt: parseUpdaterTimestamp(response.CheckedAt),
		Available: response.Available,
		Update:    UpdateViewToProto(response.Update),
		Error:     response.Error,
	}
}

func CheckResponseFromProto(message *wailsrelv1.UpdaterCheckResponse) CheckResponse {
	if message == nil {
		return CheckResponse{}
	}
	return CheckResponse{
		CheckedAt: formatUpdaterTimestamp(message.CheckedAt),
		Available: message.Available,
		Update:    UpdateViewFromProto(message.Update),
		Error:     message.Error,
	}
}

func ActionResponseToProto(response ActionResponse) *wailsrelv1.UpdaterActionResponse {
	return &wailsrelv1.UpdaterActionResponse{
		StartedAt: parseUpdaterTimestamp(response.StartedAt),
		Applied:   response.Applied,
		Restarted: response.Restarted,
		Message:   response.Message,
		Error:     response.Error,
	}
}

func ActionResponseFromProto(message *wailsrelv1.UpdaterActionResponse) ActionResponse {
	if message == nil {
		return ActionResponse{}
	}
	return ActionResponse{
		StartedAt: formatUpdaterTimestamp(message.StartedAt),
		Applied:   message.Applied,
		Restarted: message.Restarted,
		Message:   message.Message,
		Error:     message.Error,
	}
}

func LogEventToProto(event LogEvent) *wailsrelv1.UpdaterLogEvent {
	return &wailsrelv1.UpdaterLogEvent{
		Level:   event.Level,
		Message: event.Message,
		At:      parseUpdaterTimestamp(event.At),
	}
}

func LogEventFromProto(message *wailsrelv1.UpdaterLogEvent) LogEvent {
	if message == nil {
		return LogEvent{}
	}
	return LogEvent{
		Level:   message.Level,
		Message: message.Message,
		At:      formatUpdaterTimestamp(message.At),
	}
}

func ProgressEventToProto(event ProgressEvent) *wailsrelv1.UpdaterProgressEvent {
	return &wailsrelv1.UpdaterProgressEvent{
		Downloaded: event.Downloaded,
		Total:      event.Total,
	}
}

func ProgressEventFromProto(message *wailsrelv1.UpdaterProgressEvent) ProgressEvent {
	if message == nil {
		return ProgressEvent{}
	}
	return ProgressEvent{
		Downloaded: message.Downloaded,
		Total:      message.Total,
	}
}

func RelaunchRequestToProto(request RelaunchRequest) *wailsrelv1.UpdaterRelaunchRequest {
	return &wailsrelv1.UpdaterRelaunchRequest{
		Executable: request.Executable,
		Args:       append([]string(nil), request.Args...),
	}
}

func RelaunchRequestFromProto(message *wailsrelv1.UpdaterRelaunchRequest) RelaunchRequest {
	if message == nil {
		return RelaunchRequest{}
	}
	return RelaunchRequest{
		Executable: message.Executable,
		Args:       append([]string(nil), message.Args...),
	}
}

func FrontendBundleViewToProto(view *FrontendBundleView) *wailsrelv1.UpdaterFrontendBundleView {
	if view == nil {
		return nil
	}
	return &wailsrelv1.UpdaterFrontendBundleView{
		Kind:         view.Kind,
		Name:         view.Name,
		Version:      view.Version,
		CompatId:     view.CompatID,
		Channel:      view.Channel,
		SourceBranch: view.SourceBranch,
		CommitSha:    view.CommitSHA,
	}
}

func FrontendBundleViewFromProto(message *wailsrelv1.UpdaterFrontendBundleView) *FrontendBundleView {
	if message == nil {
		return nil
	}
	return &FrontendBundleView{
		Kind:         message.Kind,
		Name:         message.Name,
		Version:      message.Version,
		CompatID:     message.CompatId,
		Channel:      message.Channel,
		SourceBranch: message.SourceBranch,
		CommitSHA:    message.CommitSha,
	}
}

func FrontendCodepushViewToProto(view *FrontendCodepushView) *wailsrelv1.UpdaterFrontendCodepushView {
	if view == nil {
		return nil
	}
	return &wailsrelv1.UpdaterFrontendCodepushView{
		Name:        view.Name,
		Version:     view.Version,
		CompatId:    view.CompatID,
		Url:         view.URL,
		Checksum:    view.Checksum,
		Size:        view.Size,
		Force:       view.Force,
		PublishedAt: parseUpdaterTimestamp(view.PublishedAt),
	}
}

func FrontendCodepushViewFromProto(message *wailsrelv1.UpdaterFrontendCodepushView) *FrontendCodepushView {
	if message == nil {
		return nil
	}
	return &FrontendCodepushView{
		Name:        message.Name,
		Version:     message.Version,
		CompatID:    message.CompatId,
		URL:         message.Url,
		Checksum:    message.Checksum,
		Size:        message.Size,
		Force:       message.Force,
		PublishedAt: formatUpdaterTimestamp(message.PublishedAt),
	}
}

func FrontendExperimentViewToProto(view *FrontendExperimentView) *wailsrelv1.UpdaterFrontendExperimentView {
	if view == nil {
		return nil
	}
	return &wailsrelv1.UpdaterFrontendExperimentView{
		Name:        view.Name,
		Version:     view.Version,
		CompatId:    view.CompatID,
		Url:         view.URL,
		Checksum:    view.Checksum,
		Size:        view.Size,
		DisplayName: view.DisplayName,
		Description: view.Description,
		PublishedAt: parseUpdaterTimestamp(view.PublishedAt),
	}
}

func FrontendExperimentViewFromProto(message *wailsrelv1.UpdaterFrontendExperimentView) *FrontendExperimentView {
	if message == nil {
		return nil
	}
	return &FrontendExperimentView{
		Name:        message.Name,
		Version:     message.Version,
		CompatID:    message.CompatId,
		URL:         message.Url,
		Checksum:    message.Checksum,
		Size:        message.Size,
		DisplayName: message.DisplayName,
		Description: message.Description,
		PublishedAt: formatUpdaterTimestamp(message.PublishedAt),
	}
}

func FrontendStateToProto(state FrontendState) *wailsrelv1.UpdaterFrontendState {
	message := &wailsrelv1.UpdaterFrontendState{
		Enabled:           state.Enabled,
		CatalogUrl:        state.CatalogURL,
		LastCheckedAt:     parseUpdaterTimestamp(state.LastCheckedAt),
		LastError:         state.LastError,
		Offline:           state.Offline,
		Stale:             state.Stale,
		ActiveMode:        state.ActiveMode,
		Selection:         state.Selection,
		ActiveBundle:      FrontendBundleViewToProto(state.ActiveBundle),
		InstalledCodepush: FrontendBundleViewToProto(state.InstalledCodepush),
		AvailableCodepush: FrontendCodepushViewToProto(state.AvailableCodepush),
	}
	if len(state.InstalledExperiments) > 0 {
		message.InstalledExperiments = make([]*wailsrelv1.UpdaterFrontendBundleView, 0, len(state.InstalledExperiments))
		for _, entry := range state.InstalledExperiments {
			message.InstalledExperiments = append(message.InstalledExperiments, FrontendBundleViewToProto(&entry))
		}
	}
	if len(state.AvailableExperiments) > 0 {
		message.AvailableExperiments = make([]*wailsrelv1.UpdaterFrontendExperimentView, 0, len(state.AvailableExperiments))
		for _, entry := range state.AvailableExperiments {
			message.AvailableExperiments = append(message.AvailableExperiments, FrontendExperimentViewToProto(&entry))
		}
	}
	return message
}

func FrontendStateFromProto(message *wailsrelv1.UpdaterFrontendState) FrontendState {
	if message == nil {
		return FrontendState{}
	}
	state := FrontendState{
		Enabled:           message.Enabled,
		CatalogURL:        message.CatalogUrl,
		LastCheckedAt:     formatUpdaterTimestamp(message.LastCheckedAt),
		LastError:         message.LastError,
		Offline:           message.Offline,
		Stale:             message.Stale,
		ActiveMode:        message.ActiveMode,
		Selection:         message.Selection,
		ActiveBundle:      FrontendBundleViewFromProto(message.ActiveBundle),
		InstalledCodepush: FrontendBundleViewFromProto(message.InstalledCodepush),
		AvailableCodepush: FrontendCodepushViewFromProto(message.AvailableCodepush),
	}
	if len(message.InstalledExperiments) > 0 {
		state.InstalledExperiments = make([]FrontendBundleView, 0, len(message.InstalledExperiments))
		for _, entry := range message.InstalledExperiments {
			if view := FrontendBundleViewFromProto(entry); view != nil {
				state.InstalledExperiments = append(state.InstalledExperiments, *view)
			}
		}
	}
	if len(message.AvailableExperiments) > 0 {
		state.AvailableExperiments = make([]FrontendExperimentView, 0, len(message.AvailableExperiments))
		for _, entry := range message.AvailableExperiments {
			if view := FrontendExperimentViewFromProto(entry); view != nil {
				state.AvailableExperiments = append(state.AvailableExperiments, *view)
			}
		}
	}
	return state
}

func cloneUpdaterMetadata(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func parseUpdaterTimestamp(value string) *timestamppb.Timestamp {
	if parsed, ok := parseUpdaterTime(value); ok {
		return contract.Timestamp(parsed)
	}
	return nil
}

func formatUpdaterTimestamp(value *timestamppb.Timestamp) string {
	parsed := contract.TimeValue(value)
	if parsed.IsZero() {
		return ""
	}
	return parsed.Format(time.RFC3339)
}

func parseUpdaterTime(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed.UTC(), true
}
