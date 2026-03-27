package ingr

import (
	"bytes"
	"io"
	"strings"
)

// Unmarshal parses INGR-encoded data and stores the result in v.
// See Decoder.Decode for supported target types.
func Unmarshal(data []byte, v any) error {
	dec := NewDecoder(bytes.NewReader(data))
	return dec.Decode(v)
}

// UnmarshalString is a convenience wrapper around Unmarshal that accepts
// a string instead of a byte slice.
func UnmarshalString(s string, v any) error {
	dec := NewDecoder(strings.NewReader(s))
	return dec.Decode(v)
}

// Validate checks whether data is a well-formed INGR file. It returns a
// *SyntaxError describing the first problem found, or nil if valid.
func Validate(data []byte) error {
	dec := NewDecoder(bytes.NewReader(data))
	if _, err := dec.ReadHeader(); err != nil {
		return err
	}
	for dec.More() {
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}
	if dec.err != nil {
		return dec.err
	}
	return nil
}
