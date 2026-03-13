package contract

import (
	"fmt"
	"mime"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	ContentTypeJSON     = "application/json"
	ContentTypeProtobuf = "application/x-protobuf"
)

var (
	jsonMarshalOptions = protojson.MarshalOptions{
		UseProtoNames:   true,
		EmitUnpopulated: false,
		Multiline:       true,
		Indent:          "  ",
	}
	jsonUnmarshalOptions = protojson.UnmarshalOptions{
		DiscardUnknown: true,
	}
	protoMarshalOptions = proto.MarshalOptions{
		Deterministic: true,
	}
)

func NormalizeContentType(value string) string {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(value))
	if err != nil {
		return strings.TrimSpace(value)
	}
	return mediaType
}

func PreferredAcceptHeader() string {
	return ContentTypeProtobuf + ", " + ContentTypeJSON + ";q=0.9"
}

func IsProtobufContentType(value string) bool {
	return NormalizeContentType(value) == ContentTypeProtobuf
}

func MarshalJSON(msg proto.Message) ([]byte, error) {
	data, err := jsonMarshalOptions.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func MarshalJSONCompact(msg proto.Message) ([]byte, error) {
	return protojson.MarshalOptions{
		UseProtoNames:   true,
		EmitUnpopulated: false,
	}.Marshal(msg)
}

func MarshalProtobuf(msg proto.Message) ([]byte, error) {
	return protoMarshalOptions.Marshal(msg)
}

func Unmarshal(data []byte, contentType string, msg proto.Message) error {
	if IsProtobufContentType(contentType) {
		if err := proto.Unmarshal(data, msg); err == nil {
			return nil
		}
	}
	if err := jsonUnmarshalOptions.Unmarshal(data, msg); err == nil {
		return nil
	}
	if err := proto.Unmarshal(data, msg); err == nil {
		return nil
	}
	return fmt.Errorf("decode contract payload as %s", NormalizeContentType(contentType))
}

func Marshal(msg proto.Message, contentType string) ([]byte, error) {
	if IsProtobufContentType(contentType) {
		return MarshalProtobuf(msg)
	}
	return MarshalJSON(msg)
}

func Timestamp(value time.Time) *timestamppb.Timestamp {
	if value.IsZero() {
		return nil
	}
	return timestamppb.New(value.UTC())
}

func TimeValue(value *timestamppb.Timestamp) time.Time {
	if value == nil {
		return time.Time{}
	}
	return value.AsTime().UTC()
}
