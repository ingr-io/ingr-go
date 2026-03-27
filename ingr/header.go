package ingr

import (
	"fmt"
	"strings"
)

// Column describes a single column in an INGR header.
type Column struct {
	Name string // column name, e.g. "$ID", "name"
	Type string // optional type annotation, e.g. "string", "int"; empty if untyped
}

// Header carries the metadata parsed from line 1 of an INGR file.
type Header struct {
	Recordset string   // e.g. "people"
	Columns   []Column // ordered list of columns
}

// parseHeader parses the first line of an INGR file into a Header.
//
// The required format is:
//
//	# INGR.io | recordset: $ID[:type], col2[:type], ...
//
// The '|' separator is mandatory. Spaces around '#' and '|' are optional.
func parseHeader(line string) (Header, error) {
	var h Header
	s := line

	// Strip leading '#' and whitespace
	if !strings.HasPrefix(s, "#") {
		return h, fmt.Errorf("header must start with '#'")
	}
	s = s[1:]
	s = strings.TrimLeft(s, " \t")

	// Expect "INGR.io"
	if !strings.HasPrefix(s, "INGR.io") {
		return h, fmt.Errorf("header must contain 'INGR.io'")
	}
	s = s[len("INGR.io"):]
	s = strings.TrimLeft(s, " \t")

	// Mandatory "|"
	if !strings.HasPrefix(s, "|") {
		return h, fmt.Errorf("header must contain '|' after 'INGR.io'")
	}
	s = s[1:]
	s = strings.TrimLeft(s, " \t")

	// Now expect "recordset_name: col1, col2, ..."
	colonIdx := strings.Index(s, ":")
	if colonIdx < 0 {
		return h, fmt.Errorf("header missing ':' after recordset name")
	}

	h.Recordset = strings.TrimSpace(s[:colonIdx])
	if h.Recordset == "" {
		return h, fmt.Errorf("empty recordset name in header")
	}

	// Parse comma-separated columns after the first colon.
	colsPart := s[colonIdx+1:]
	cols := strings.Split(colsPart, ",")
	for _, raw := range cols {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		// Split on first ":" to separate name from optional type.
		// Using Cut (Go 1.18+) so that complex types like "map[string]int"
		// are preserved in the type portion.
		name, typ, hasType := strings.Cut(raw, ":")
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		col := Column{Name: name}
		if hasType {
			col.Type = strings.TrimSpace(typ)
		}
		h.Columns = append(h.Columns, col)
	}

	if len(h.Columns) == 0 {
		return h, fmt.Errorf("header has no columns")
	}

	return h, nil
}

// String formats the Header back into a valid INGR header line.
func (h Header) String() string {
	var sb strings.Builder
	sb.WriteString("# INGR.io | ")
	sb.WriteString(h.Recordset)
	sb.WriteString(": ")
	for i, col := range h.Columns {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(col.Name)
		if col.Type != "" {
			sb.WriteByte(':')
			sb.WriteString(col.Type)
		}
	}
	return sb.String()
}
