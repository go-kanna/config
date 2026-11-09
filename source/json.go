package source

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
)

// space is what JSON skips between its tokens.
const space = " \t\r\n"

// number is a JSON number as it is written.
type number string

func (n number) String() string { return string(n) }

// keepNumbers reads a number into an any as it is written, so that an integer
// too big for a float64 reads as it is.
var keepNumbers = json.WithUnmarshalers(json.UnmarshalFromFunc(func(dec *jsontext.Decoder, v *any) error {
	if dec.PeekKind() != '0' {
		return errors.ErrUnsupported
	}
	val, err := dec.ReadValue()
	if err != nil {
		return err //nolint:wrapcheck // json/v2 reports a syntactic error where it is
	}
	*v = number(val)
	return nil
}))

// JSON reads a JSON object into its sections with encoding/json/v2, which
// rejects a name an object holds twice and text that is not UTF-8. A number
// keeps the text it is written with. Its error says where the data is wrong, but not what encoding/json/v2
// says of it, which may quote a secret written there.
func JSON(data []byte) (map[string]any, error) {
	if len(bytes.TrimLeft(data, space)) == 0 {
		return nil, errors.New("not valid JSON")
	}
	var tree map[string]any
	if err := json.Unmarshal(data, &tree, keepNumbers); err != nil {
		return nil, jsonError(data, err)
	}
	if tree == nil {
		return map[string]any{}, nil
	}
	return tree, nil
}

func jsonError(data []byte, err error) error {
	if se, ok := errors.AsType[*jsontext.SyntacticError](err); ok {
		line, col := position(data, int(se.ByteOffset))
		if errors.Is(err, jsontext.ErrDuplicateName) {
			return fmt.Errorf("line %d, column %d repeats a name of its object", line, col)
		}
		return fmt.Errorf("line %d, column %d is not valid JSON", line, col)
	}
	if _, ok := errors.AsType[*json.SemanticError](err); ok {
		line, col := position(data, len(data)-len(bytes.TrimLeft(data, space)))
		return fmt.Errorf("line %d, column %d is not a JSON object", line, col)
	}
	return errors.New("not valid JSON")
}

// position returns the line and the column of byte i of data, both from 1.
func position(data []byte, i int) (int, int) {
	before := data[:min(max(i, 0), len(data))]
	return bytes.Count(before, []byte{'\n'}) + 1, len(before) - bytes.LastIndexByte(before, '\n')
}
