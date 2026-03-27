package ingr

import (
	"fmt"
	"reflect"
)

// SyntaxError represents a syntax error encountered while parsing an INGR file.
type SyntaxError struct {
	Offset int64  // byte offset where the error was detected
	Line   int    // 1-based line number
	msg    string // human-readable description
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("ingr: %s (line %d, offset %d)", e.msg, e.Line, e.Offset)
}

// UnmarshalTypeError describes a JSON value that could not be assigned
// to a Go value of a particular type.
type UnmarshalTypeError struct {
	Value  string       // JSON value string (e.g. "number", "string")
	Type   reflect.Type // Go type that could not be assigned to
	Offset int64        // byte offset where the error was detected
	Line   int          // 1-based line number
	Field  string       // INGR column name (empty if not applicable)
}

func (e *UnmarshalTypeError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("ingr: cannot unmarshal %s into Go struct field .%s of type %s (line %d)",
			e.Value, e.Field, e.Type, e.Line)
	}
	return fmt.Sprintf("ingr: cannot unmarshal %s into Go value of type %s (line %d)",
		e.Value, e.Type, e.Line)
}
