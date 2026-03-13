package manifest

import (
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
	reposchema "github.com/you/wailsrel/schema"
)

const manifestSchemaName = "manifest.schema.json"

func Validate(m *ReleaseManifest) []error {
	if m == nil {
		return []error{fmt.Errorf("manifest is required")}
	}

	schemaData, err := reposchema.ReadFile(manifestSchemaName)
	if err != nil {
		return []error{err}
	}

	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(manifestSchemaName, anyJSON(schemaData)); err != nil {
		return []error{err}
	}
	schema, err := compiler.Compile(manifestSchemaName)
	if err != nil {
		return []error{err}
	}
	instance, err := anyJSONValue(m)
	if err != nil {
		return []error{err}
	}
	if err := schema.Validate(instance); err != nil {
		return []error{err}
	}
	return nil
}

func anyJSON(data []byte) any {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		panic(fmt.Sprintf("invalid embedded JSON schema: %v", err))
	}
	return value
}

func anyJSONValue(value any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}
