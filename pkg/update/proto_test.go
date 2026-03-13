package update

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Eriyc/wailsrel/gen/go/wailsrel/v1"
	"github.com/Eriyc/wailsrel/pkg/contract"
)

func TestCheckContractsJSONAndProtobufRoundTrip(t *testing.T) {
	originalOpts := CheckOpts{
		CurrentVersion: "1.2.3",
		CurrentHash:    "sha256:" + strings.Repeat("a", 64),
		NativeCompat:   "2",
		Channel:        "beta",
		ManifestURL:    "https://updates.example.com/manifest.pb",
	}
	if roundTrip := CheckOptsFromProto(CheckOptsToProto(originalOpts)); !reflect.DeepEqual(roundTrip, originalOpts) {
		t.Fatalf("check opts round-trip mismatch: %+v != %+v", roundTrip, originalOpts)
	}

	originalResult := &CheckResult{
		Available: true,
		Native: &UpdateInfo{
			Version:       "2.0.0",
			Channel:       "stable",
			ReleaseNotes:  "Security update",
			Mandatory:     true,
			ArtifactURL:   "https://updates.example.com/download/app.bin",
			ArtifactHash:  "sha256:" + strings.Repeat("b", 64),
			ArtifactSize:  2048,
			DeltaURL:      "https://updates.example.com/download/app.patch",
			DeltaHash:     "sha256:" + strings.Repeat("c", 64),
			DeltaSize:     512,
			DeltaFromHash: "sha256:" + strings.Repeat("d", 64),
			Frontend: &FrontendUpdateInfo{
				Channel:  "stable",
				Version:  "2.0.1",
				CompatID: "2",
				URL:      "https://updates.example.com/download/frontend.zip",
				Hash:     "sha256:" + strings.Repeat("e", 64),
				Size:     1024,
			},
		},
		Frontend: &FrontendUpdateInfo{
			Channel:  "stable",
			Version:  "2.0.1",
			CompatID: "2",
			URL:      "https://updates.example.com/download/frontend.zip",
			Hash:     "sha256:" + strings.Repeat("e", 64),
			Size:     1024,
		},
	}

	jsonMessage := CheckResultToProto(originalResult)
	jsonData, err := contract.MarshalJSON(jsonMessage)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	var jsonDecodedMessage wailsrelv1.UpdateCheckResult
	if err := contract.Unmarshal(jsonData, contract.ContentTypeJSON, &jsonDecodedMessage); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	jsonDecoded := CheckResultFromProto(&jsonDecodedMessage)

	protoData, err := contract.MarshalProtobuf(jsonMessage)
	if err != nil {
		t.Fatalf("marshal protobuf: %v", err)
	}
	var protoDecodedMessage wailsrelv1.UpdateCheckResult
	if err := contract.Unmarshal(protoData, contract.ContentTypeProtobuf, &protoDecodedMessage); err != nil {
		t.Fatalf("unmarshal protobuf: %v", err)
	}
	protoDecoded := CheckResultFromProto(&protoDecodedMessage)

	if !reflect.DeepEqual(jsonDecoded, protoDecoded) {
		t.Fatalf("check result round-trip mismatch\njson=%+v\nproto=%+v", jsonDecoded, protoDecoded)
	}
	if !reflect.DeepEqual(protoDecoded, originalResult) {
		t.Fatalf("protobuf round-trip lost data\nwant=%+v\ngot=%+v", originalResult, protoDecoded)
	}
}
