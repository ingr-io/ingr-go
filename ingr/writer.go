package ingr

import (
	"crypto/sha256"
	"fmt"
	"hash"
	"io"
	"log"
	"strings"
)

type RecordsWriter interface {
	WriteRecords(recordsDelimiter int, r ...Record) (n int, err error)
	io.Closer
}

type ColDef struct {
	Name string `json:"name"`           // required
	Type string `json:"type,omitempty"` // optional
}

type HashAlgorithm string

const (
	SHA256 HashAlgorithm = "sha256"
)

func NewRecordsWriter(w io.Writer, hashAlg ...HashAlgorithm) RecordsWriter {
	if w == nil {
		panic("w io.Writer cannot be nil")
	}
	rw := &recordsWriter{w: w}
	if len(hashAlg) > 0 {
		rw.hashAlg = hashAlg[0]
		switch rw.hashAlg {
		case SHA256:
			rw.hash = sha256.New()
		default:
			panic(fmt.Sprintf("unsupported hash algorithm: %s", rw.hashAlg))
		}
	}
	return rw
}

type recordsWriter struct {
	w       io.Writer
	cols    []ColDef
	hashAlg HashAlgorithm
	hash    hash.Hash

	headerWritten bool

	recordsCount int
}

func (rw *recordsWriter) WriteHeader(title string) (n int, err error) {
	if rw.headerWritten {
		return 0, fmt.Errorf("header already written")
	}
	if n, err = rw.w.Write([]byte(fmt.Sprintf("# INGR.io | %s : ", title))); err != nil {
		if n > 0 {
			rw.headerWritten = true
		}
		return n, fmt.Errorf("failed to write header: %w", err)
	}
	rw.headerWritten = true

	for i, col := range rw.cols {
		var m int
		m, err = rw.w.Write([]byte(col.Name))
		if err != nil {
			return 0, fmt.Errorf("failed to write column name at index %d: %w", i, err)
		}
		n += m

	}
	return n, nil
}

func (rw *recordsWriter) WriteRecords(recordsDelimiter int, records ...Record) (n int, err error) {
	if !rw.headerWritten {
		log.Fatal("header must be written before records")
	}
	var b int

	for _, r := range records {
		for _, c := range rw.cols {
			b, err = rw.w.Write([]byte(fmt.Sprintf("%v", r.GetValue(c.Name))))
			n += b
			if err != nil {
				return n, fmt.Errorf("failed to write column name %s: %w", c.Name, err)
			}
		}
		if recordsDelimiter > 0 {
			b, err = rw.w.Write([]byte("#" + strings.Repeat("-", recordsDelimiter) + "\n"))
			n += b
			if err != nil {
				return n, fmt.Errorf("failed to write records delimiter: %w", err)
			}
		}
	}
	return n, nil
}

func (rw *recordsWriter) writeFooter() (n int, err error) {
	// Write record count line — always with trailing newline
	var i int
	i, err = fmt.Fprintf(rw.w, "# %d record(s)\n", rw.recordsCount)
	n += i
	if err != nil {
		return n, fmt.Errorf("failed to write records count: %w", err)
	}
	if rw.hash != nil {
		i, err = fmt.Fprintf(rw.w, "# %s:%x", rw.hashAlg, rw.hash)
		n += i
	}
	return n, err
}

func (rw *recordsWriter) Close() error {
	_, err := rw.writeFooter()
	return err
}
