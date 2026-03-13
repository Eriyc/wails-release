package frontend

import (
	"encoding/json"

	"github.com/Eriyc/wailsrel/gen/go/wailsrel/v1"
	"github.com/Eriyc/wailsrel/pkg/contract"
)

func EncodeCatalogJSON(catalog Catalog) ([]byte, error) {
	data, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func EncodeCatalogProtobuf(catalog Catalog) ([]byte, error) {
	return contract.MarshalProtobuf(catalogToProto(catalog))
}

func DecodeCatalogResponse(data []byte, contentType, publicKey, expectedAppID string) (*Catalog, error) {
	message := &wailsrelv1.FrontendCatalog{}
	if err := contract.Unmarshal(data, contentType, message); err != nil {
		return nil, err
	}
	catalog := catalogFromProto(message)
	if err := catalog.Verify(publicKey, expectedAppID); err != nil {
		return nil, err
	}
	return &catalog, nil
}

func CanonicalCatalogPayload(catalog Catalog) ([]byte, error) {
	return catalog.canonicalSignedPayload()
}

func (c Catalog) canonicalSignedPayload() ([]byte, error) {
	message := catalogToProto(c)
	message.Signature = ""
	message.SignedAt = contract.Timestamp(c.SignedAt)
	return contract.MarshalProtobuf(message)
}

func catalogToProto(catalog Catalog) *wailsrelv1.FrontendCatalog {
	out := &wailsrelv1.FrontendCatalog{
		SchemaVersion: int32(catalog.SchemaVersion),
		AppId:         catalog.AppID,
		GeneratedAt:   contract.Timestamp(catalog.GeneratedAt),
		Signature:     catalog.Signature,
		SignedAt:      contract.Timestamp(catalog.SignedAt),
		Codepush:      make([]*wailsrelv1.FrontendCatalogBundle, 0, len(catalog.Codepush)),
		Experiments:   make([]*wailsrelv1.FrontendCatalogBundle, 0, len(catalog.Experiments)),
	}
	for _, entry := range catalog.Codepush {
		out.Codepush = append(out.Codepush, &wailsrelv1.FrontendCatalogBundle{
			Name:        entry.Name,
			Version:     entry.Version,
			CompatId:    entry.CompatID,
			Url:         entry.URL,
			Checksum:    entry.Checksum,
			Size:        entry.Size,
			Force:       entry.Force,
			PublishedAt: contract.Timestamp(entry.PublishedAt),
		})
	}
	for _, entry := range catalog.Experiments {
		out.Experiments = append(out.Experiments, &wailsrelv1.FrontendCatalogBundle{
			Name:        entry.Name,
			Version:     entry.Version,
			CompatId:    entry.CompatID,
			Url:         entry.URL,
			Checksum:    entry.Checksum,
			Size:        entry.Size,
			PublishedAt: contract.Timestamp(entry.PublishedAt),
			DisplayName: entry.DisplayName,
			Description: entry.Description,
		})
	}
	return out
}

func catalogFromProto(message *wailsrelv1.FrontendCatalog) Catalog {
	out := Catalog{
		SchemaVersion: int(message.SchemaVersion),
		AppID:         message.AppId,
		GeneratedAt:   contract.TimeValue(message.GeneratedAt),
		Signature:     message.Signature,
		SignedAt:      contract.TimeValue(message.SignedAt),
		Codepush:      make([]CodepushEntry, 0, len(message.Codepush)),
		Experiments:   make([]ExperimentEntry, 0, len(message.Experiments)),
	}
	for _, entry := range message.Codepush {
		if entry == nil {
			continue
		}
		out.Codepush = append(out.Codepush, CodepushEntry{
			Name:        entry.Name,
			Version:     entry.Version,
			CompatID:    entry.CompatId,
			URL:         entry.Url,
			Checksum:    entry.Checksum,
			Size:        entry.Size,
			Force:       entry.Force,
			PublishedAt: contract.TimeValue(entry.PublishedAt),
		})
	}
	for _, entry := range message.Experiments {
		if entry == nil {
			continue
		}
		out.Experiments = append(out.Experiments, ExperimentEntry{
			Name:        entry.Name,
			Version:     entry.Version,
			CompatID:    entry.CompatId,
			URL:         entry.Url,
			Checksum:    entry.Checksum,
			Size:        entry.Size,
			DisplayName: entry.DisplayName,
			Description: entry.Description,
			PublishedAt: contract.TimeValue(entry.PublishedAt),
		})
	}
	return out
}
