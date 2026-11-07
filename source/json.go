package source

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// space is what JSON skips between its tokens.
const space = " \t\r\n"

// JSON reads a JSON object into its sections. A number keeps the text it is
// written with, so that an integer too big for a float64 reads as it is. Its
// error says where the data is wrong, but not what encoding/json says of it,
// which may quote a secret written there.
func JSON(data []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var tree map[string]any
	if err := dec.Decode(&tree); err != nil {
		return nil, jsonError(data, err)
	}
	if rest := bytes.TrimLeft(data[dec.InputOffset():], space); len(rest) > 0 {
		return nil, invalidJSON(data, len(data)-len(rest))
	}
	if tree == nil {
		return map[string]any{}, nil
	}
	return tree, nil
}

func jsonError(data []byte, err error) error {
	if se, ok := errors.AsType[*json.SyntaxError](err); ok {
		return invalidJSON(data, int(se.Offset)-1)
	}
	if _, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
		line, col := position(data, len(data)-len(bytes.TrimLeft(data, space)))
		return fmt.Errorf("line %d, column %d is not a JSON object", line, col)
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return invalidJSON(data, len(data))
	}
	return errors.New("not valid JSON")
}

// invalidJSON reports data as wrong at its byte i.
func invalidJSON(data []byte, i int) error {
	line, col := position(data, i)
	return fmt.Errorf("line %d, column %d is not valid JSON", line, col)
}

// position returns the line and the column of byte i of data, both from 1.
func position(data []byte, i int) (int, int) {
	before := data[:min(max(i, 0), len(data))]
	return bytes.Count(before, []byte{'\n'}) + 1, len(before) - bytes.LastIndexByte(before, '\n')
}
