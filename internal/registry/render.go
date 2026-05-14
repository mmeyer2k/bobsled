// Package registry renders zot config and podman client config from inventory.
package registry

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/template"

	"github.com/m-meyer2k/bobsled/assets"
	"github.com/m-meyer2k/bobsled/internal/inventory"
)

// RenderConfig produces a zot config.json from the given inventory.Registry.
// Output is normalized through json.Marshal so whitespace is stable for tests.
func RenderConfig(r *inventory.Registry) ([]byte, error) {
	t, err := template.New("zot").Parse(assets.RegistryConfigTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse zot template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, r); err != nil {
		return nil, fmt.Errorf("execute zot template: %w", err)
	}
	// Re-marshal through encoding/json to normalize whitespace; this also
	// validates that the rendered config is valid JSON.
	var any interface{}
	if err := json.Unmarshal(buf.Bytes(), &any); err != nil {
		return nil, fmt.Errorf("rendered config is not valid JSON: %w\n%s", err, buf.String())
	}
	return json.MarshalIndent(any, "", "  ")
}
