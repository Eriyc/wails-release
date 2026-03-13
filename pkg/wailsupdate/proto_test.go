package wailsupdate

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Eriyc/wailsrel/gen/go/wailsrel/v1"
	"github.com/Eriyc/wailsrel/pkg/contract"
)

func TestServiceContractsJSONAndProtobufRoundTrip(t *testing.T) {
	state := State{
		ManifestURL:    "https://updates.example.com/manifest.pb",
		CurrentVersion: "1.2.3",
		CurrentHash:    "sha256:" + strings.Repeat("a", 64),
		Channel:        "beta",
		NativeCompat:   "2",
		TargetPath:     "/Applications/MyApp.app/Contents/MacOS/MyApp",
		TempDir:        "/tmp/myapp-update",
		LastCheckedAt:  "2026-03-13T12:00:00Z",
		PendingRestart: true,
		AvailableUpdate: &UpdateView{
			Version:       "2.0.0",
			Channel:       "beta",
			ReleaseNotes:  "Hotfix",
			Mandatory:     true,
			ArtifactURL:   "https://updates.example.com/download/app.bin",
			ArtifactHash:  "sha256:" + strings.Repeat("b", 64),
			ArtifactSize:  2048,
			DeltaURL:      "https://updates.example.com/download/app.patch",
			DeltaHash:     "sha256:" + strings.Repeat("c", 64),
			DeltaSize:     512,
			DeltaFromHash: "sha256:" + strings.Repeat("d", 64),
			FrontendURL:   "https://updates.example.com/download/frontend.zip",
			FrontendHash:  "sha256:" + strings.Repeat("e", 64),
			FrontendSize:  1024,
			FrontendOnly:  false,
		},
		Metadata:  map[string]string{"repository": "owner/repo"},
		Notes:     []string{"checksum unavailable"},
		LastError: "network timeout",
	}
	if roundTrip := StateFromProto(StateToProto(state)); !reflect.DeepEqual(roundTrip, state) {
		t.Fatalf("state round-trip mismatch\nwant=%+v\ngot=%+v", state, roundTrip)
	}

	frontendState := FrontendState{
		Enabled:       true,
		CatalogURL:    "https://updates.example.com/frontend/catalog.pb",
		LastCheckedAt: "2026-03-13T12:05:00Z",
		LastError:     "offline",
		Offline:       true,
		Stale:         true,
		ActiveMode:    "experiment",
		Selection:     "green",
		ActiveBundle: &FrontendBundleView{
			Kind:         "experiment",
			Name:         "green",
			Version:      "1.2.4",
			CompatID:     "2",
			Channel:      "beta",
			SourceBranch: "main",
			CommitSHA:    "abc123",
		},
		InstalledCodepush: &FrontendBundleView{
			Kind:      "codepush",
			Name:      "hotfix-1",
			Version:   "1.2.5",
			CompatID:  "2",
			Channel:   "stable",
			CommitSHA: "def456",
		},
		InstalledExperiments: []FrontendBundleView{
			{
				Kind:      "experiment",
				Name:      "green",
				Version:   "1.2.4",
				CompatID:  "2",
				Channel:   "beta",
				CommitSHA: "abc123",
			},
		},
		AvailableCodepush: &FrontendCodepushView{
			Name:        "hotfix-2",
			Version:     "1.2.6",
			CompatID:    "2",
			URL:         "https://updates.example.com/download/hotfix-2.zip",
			Checksum:    "sha256:" + strings.Repeat("f", 64),
			Size:        1234,
			Force:       true,
			PublishedAt: "2026-03-13T12:06:00Z",
		},
		AvailableExperiments: []FrontendExperimentView{
			{
				Name:        "blue",
				Version:     "1.2.6",
				CompatID:    "2",
				URL:         "https://updates.example.com/download/blue.zip",
				Checksum:    "sha256:" + strings.Repeat("1", 64),
				Size:        2234,
				DisplayName: "Blue",
				Description: "Blue experiment",
				PublishedAt: "2026-03-13T12:07:00Z",
			},
		},
	}
	if roundTrip := FrontendStateFromProto(FrontendStateToProto(frontendState)); !reflect.DeepEqual(roundTrip, frontendState) {
		t.Fatalf("frontend state round-trip mismatch\nwant=%+v\ngot=%+v", frontendState, roundTrip)
	}

	cases := []struct {
		name  string
		msg   any
		proto func(any) any
	}{
		{
			name: "check response",
			msg: CheckResponse{
				CheckedAt: "2026-03-13T12:10:00Z",
				Available: true,
				Update: &UpdateView{
					Version:      "2.0.0",
					Channel:      "stable",
					FrontendOnly: true,
					FrontendURL:  "https://updates.example.com/download/frontend.zip",
					FrontendHash: "sha256:" + strings.Repeat("2", 64),
					FrontendSize: 2048,
				},
				Error: "none",
			},
			proto: func(value any) any { return CheckResponseFromProto(CheckResponseToProto(value.(CheckResponse))) },
		},
		{
			name: "action response",
			msg: ActionResponse{
				StartedAt: "2026-03-13T12:11:00Z",
				Applied:   true,
				Restarted: false,
				Message:   "Update staged successfully.",
				Error:     "",
			},
			proto: func(value any) any { return ActionResponseFromProto(ActionResponseToProto(value.(ActionResponse))) },
		},
		{
			name: "log event",
			msg: LogEvent{
				Level:   "info",
				Message: "Update 2.0.0 is available.",
				At:      "2026-03-13T12:12:00Z",
			},
			proto: func(value any) any { return LogEventFromProto(LogEventToProto(value.(LogEvent))) },
		},
		{
			name: "progress event",
			msg: ProgressEvent{
				Downloaded: 512,
				Total:      2048,
			},
			proto: func(value any) any { return ProgressEventFromProto(ProgressEventToProto(value.(ProgressEvent))) },
		},
		{
			name: "relaunch request",
			msg: RelaunchRequest{
				Executable: "/Applications/MyApp.app/Contents/MacOS/MyApp",
				Args:       []string{"--foreground"},
			},
			proto: func(value any) any { return RelaunchRequestFromProto(RelaunchRequestToProto(value.(RelaunchRequest))) },
		},
	}

	for _, tc := range cases {
		if roundTrip := tc.proto(tc.msg); !reflect.DeepEqual(roundTrip, tc.msg) {
			t.Fatalf("%s round-trip mismatch\nwant=%+v\ngot=%+v", tc.name, tc.msg, roundTrip)
		}
	}

	jsonData, err := contract.MarshalJSON(StateToProto(state))
	if err != nil {
		t.Fatalf("marshal state json: %v", err)
	}
	var jsonMessage wailsrelv1.UpdaterState
	if err := contract.Unmarshal(jsonData, contract.ContentTypeJSON, &jsonMessage); err != nil {
		t.Fatalf("unmarshal state json: %v", err)
	}

	protoData, err := contract.MarshalProtobuf(FrontendStateToProto(frontendState))
	if err != nil {
		t.Fatalf("marshal frontend state protobuf: %v", err)
	}
	var protoMessage wailsrelv1.UpdaterFrontendState
	if err := contract.Unmarshal(protoData, contract.ContentTypeProtobuf, &protoMessage); err != nil {
		t.Fatalf("unmarshal frontend state protobuf: %v", err)
	}

	if decoded := StateFromProto(&jsonMessage); !reflect.DeepEqual(decoded, state) {
		t.Fatalf("json state round-trip mismatch\nwant=%+v\ngot=%+v", state, decoded)
	}
	if decoded := FrontendStateFromProto(&protoMessage); !reflect.DeepEqual(decoded, frontendState) {
		t.Fatalf("protobuf frontend state round-trip mismatch\nwant=%+v\ngot=%+v", frontendState, decoded)
	}
}
