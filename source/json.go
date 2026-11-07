package source

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// JSON reads a JSON object into its sections. A number keeps the text it is
// written with, so that an integer too big for a float64 reads as it is.
func JSON(data []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var tree map[string]any
	if err := dec.Decode(&tree); err != nil {
		return nil, fmt.Errorf("decode JSON: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("decode JSON: more after the object")
	}
	if tree == nil {
		return map[string]any{}, nil
	}
	return tree, nil
}
