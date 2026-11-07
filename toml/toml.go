// Package toml reads a file of settings written in TOML, for a source.File of
// github.com/go-kanna/config/source:
//
//	source.File{Path: "config/production.toml", Decode: toml.Decode}
//
// It is a module of its own, so that only a program reading TOML requires the
// library that does.
package toml

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

// Decode reads a TOML document into its tables.
func Decode(data []byte) (map[string]any, error) {
	tree := map[string]any{}
	if err := toml.Unmarshal(data, &tree); err != nil {
		return nil, fmt.Errorf("decode TOML: %w", err)
	}
	return tree, nil
}
