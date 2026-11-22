package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"
)

func TestOpenAPISpecsHaveUniqueKeysAndSynchronizedWorkoutLogOperations(t *testing.T) {
	specs := []struct {
		name string
		path string
	}{
		{name: "runtime", path: "openapi.json"},
		{name: "docs json", path: "../../../docs/openapi.json"},
		{name: "docs yaml", path: "../../../docs/openapi.yaml"},
	}
	var canonical []byte
	for i, spec := range specs {
		data, err := os.ReadFile(spec.path)
		if err != nil {
			t.Fatalf("read %s OpenAPI document: %v", spec.name, err)
		}
		if err := validateUniqueJSONKeys(data); err != nil {
			t.Errorf("%s OpenAPI document contains invalid or duplicate-key JSON: %v", spec.name, err)
			continue
		}
		if i == 0 {
			canonical = data
		} else if !bytes.Equal(canonical, data) {
			t.Errorf("%s OpenAPI document is not byte-for-byte synchronized with the runtime specification", spec.name)
		}

		var document struct {
			OpenAPI string                                `json:"openapi"`
			Paths   map[string]map[string]json.RawMessage `json:"paths"`
		}
		if err := json.Unmarshal(data, &document); err != nil {
			t.Errorf("decode %s OpenAPI document: %v", spec.name, err)
			continue
		}
		if document.OpenAPI != "3.1.0" {
			t.Errorf("%s OpenAPI version = %q, want 3.1.0", spec.name, document.OpenAPI)
		}
		operations, ok := document.Paths["/workout-logs/{log_id}"]
		if !ok {
			t.Errorf("%s OpenAPI document is missing /workout-logs/{log_id}", spec.name)
			continue
		}
		for _, method := range []string{"get", "patch"} {
			if len(operations[method]) == 0 {
				t.Errorf("%s OpenAPI document is missing %s /workout-logs/{log_id}", spec.name, method)
			}
		}
	}
}

// validateUniqueJSONKeys recursively checks every object before the standard
// decoder can silently accept and overwrite duplicate mapping keys.
func validateUniqueJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := consumeJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple top-level JSON values")
		}
		return err
	}
	return nil
}

func consumeJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		keys := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key has unexpected type %T", keyToken)
			}
			if _, exists := keys[key]; exists {
				return fmt.Errorf("duplicate object key %q", key)
			}
			keys[key] = struct{}{}
			if err := consumeJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim('}') {
			return fmt.Errorf("object ended with unexpected token %v", end)
		}
	case '[':
		for decoder.More() {
			if err := consumeJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim(']') {
			return fmt.Errorf("array ended with unexpected token %v", end)
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delim)
	}
	return nil
}
