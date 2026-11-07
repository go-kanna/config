// Package yaml reads a file of settings written in YAML, for a source.File of
// github.com/go-kanna/config/source:
//
//	source.File{Path: "config/production.yaml", Decode: yaml.Decode}
//
// It is a module of its own, so that only a program reading YAML requires the
// library that does.
package yaml

import (
	"fmt"

	"go.yaml.in/yaml/v3"
)

// Decode reads a YAML mapping into its sections. An empty document has none.
func Decode(data []byte) (map[string]any, error) {
	var tree map[string]any
	if err := yaml.Unmarshal(data, &tree); err != nil {
		return nil, fmt.Errorf("decode YAML: %w", err)
	}
	if tree == nil {
		return map[string]any{}, nil
	}
	return tree, nil
}
