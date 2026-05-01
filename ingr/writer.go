package ingr

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"strings"
)

// RecordsWriter is the interface for streaming INGR output.
// Call WriteHeader once, then WriteRecords any number of times, then Close.
type RecordsWriter interface {
	WriteHeader(title string, cols []ColDef) (n int, err error)
	// WriteRecords writes records to the stream. recordsDelimiter controls the
	// optional "#-..." separator line: <= 0 disables it; any value > 0 emits
	// "#" followed by that many '-' characters (use 1 for the minimal "#-").
	WriteRecords(recordsDelimiter int, r ...Record) (n int, err error)
	io.Closer
}

// ColDef describes a single column in an INGR header.
type ColDef struct {
	Name string `json:"name"`           // required
	Type string `json:"type,omitempty"` // optional type annotation
}

// HashAlgorithm names a supported footer hash algorithm.
type HashAlgorithm string

const (
	SHA256 HashAlgorithm = "sha256"
)

// NewRecordsWriter returns a RecordsWriter that writes to w.
// Pass ingr.SHA256 to append a sha256 digest line to the footer.
// Panics if w is nil or an unsupported algorithm is given.
func NewRecordsWriter(w io.Writer, hashAlg ...HashAlgorithm) RecordsWriter {
	if w == nil {
		panic("w io.Writer cannot be nil")
	}
	rw := &recordsWriter{}
	if len(hashAlg) > 0 {
		rw.hashAlg = hashAlg[0]
		switch rw.hashAlg {
		case SHA256:
			rw.hash = sha256.New()
		default:
			panic(fmt.Sprintf("unsupported hash algorithm: %s", rw.hashAlg))
		}
		// Tee all writes to the hash so it covers every byte before the footer.
		rw.w = io.MultiWriter(w, rw.hash)
	} else {
		rw.w = w
	}
	return rw
}

type recordsWriter struct {
	w       io.Writer
	cols    []ColDef
	hashAlg HashAlgorithm
	hash    hash.Hash

	headerWritten bool
	recordsCount  int
}

// WriteHeader writes the INGR header line. Must be called once before WriteRecords.
// cols defines the ordered column list that is also used by WriteRecords.
func (rw *recordsWriter) WriteHeader(title string, cols []ColDef) (n int, err error) {
	if rw.headerWritten {
		return 0, fmt.Errorf("header already written")
	}

	var sb strings.Builder
	sb.WriteString("# INGR.io | ")
	sb.WriteString(title)
	sb.WriteString(": ")
	for i, col := range cols {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(col.Name)
		if col.Type != "" {
			sb.WriteByte(':')
			sb.WriteString(col.Type)
		}
	}
	sb.WriteByte('\n')

	n, err = rw.w.Write([]byte(sb.String()))
	if err != nil {
		return n, fmt.Errorf("failed to write header: %w", err)
	}
	rw.cols = cols
	rw.headerWritten = true
	return n, nil
}

// WriteRecords writes one or more records. Each field value is JSON-encoded on its own line.
// If recordsDelimiter > 0, a "#" followed by that many '-' characters is appended after each record.
// Returns an error if WriteHeader has not been called.
func (rw *recordsWriter) WriteRecords(recordsDelimiter int, records ...Record) (n int, err error) {
	if !rw.headerWritten {
		return 0, fmt.Errorf("WriteHeader must be called before WriteRecords")
	}
	var b int

	for _, r := range records {
		for _, c := range rw.cols {
			val, jsonErr := json.Marshal(r.GetValue(c.Name))
			if jsonErr != nil {
				return n, fmt.Errorf("failed to marshal value for column %q: %w", c.Name, jsonErr)
			}
			val = append(val, '\n')
			b, err = rw.w.Write(val)
			n += b
			if err != nil {
				return n, fmt.Errorf("failed to write value for column %q: %w", c.Name, err)
			}
		}
		if recordsDelimiter > 0 {
			b, err = rw.w.Write([]byte("#" + strings.Repeat("-", recordsDelimiter) + "\n"))
			n += b
			if err != nil {
				return n, fmt.Errorf("failed to write record delimiter: %w", err)
			}
		}
		rw.recordsCount++
	}
	return n, nil
}

// writeFooter writes the footer count line and optional hash line.
// The last line has no trailing newline per spec.
func (rw *recordsWriter) writeFooter() (n int, err error) {
	var countLine string
	if rw.recordsCount == 1 {
		countLine = "# 1 record"
	} else {
		countLine = fmt.Sprintf("# %d records", rw.recordsCount)
	}

	if rw.hash == nil {
		// Count line is the last line — no trailing newline.
		n, err = fmt.Fprint(rw.w, countLine)
		if err != nil {
			return n, fmt.Errorf("failed to write record count: %w", err)
		}
		return n, nil
	}

	// Hash line follows — count line needs a newline so the hash covers it.
	var i int
	i, err = fmt.Fprintln(rw.w, countLine)
	n += i
	if err != nil {
		return n, fmt.Errorf("failed to write record count: %w", err)
	}
	// Hash line is last — no trailing newline.
	i, err = fmt.Fprintf(rw.w, "# %s:%x", rw.hashAlg, rw.hash.Sum(nil))
	n += i
	return n, err
}

// Close writes the footer. Always defer Close to ensure the footer is written.
func (rw *recordsWriter) Close() error {
	_, err := rw.writeFooter()
	return err
}
