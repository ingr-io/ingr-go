package ingr

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// errorWriter always returns an error on Write.
type errorWriter struct{}

func (e *errorWriter) Write(_ []byte) (int, error) {
	return 0, fmt.Errorf("write error")
}

// partialErrorWriter writes exactly 1 byte then returns an error,
// allowing coverage of the "n > 0 on error" branch in WriteHeader.
type partialErrorWriter struct{}

func (p *partialErrorWriter) Write(data []byte) (int, error) {
	if len(data) > 0 {
		return 1, fmt.Errorf("partial write error")
	}
	return 0, fmt.Errorf("write error")
}

// nthWriteErrorWriter succeeds for the first (failOnWrite-1) Write calls,
// then returns an error on the failOnWrite-th call.
type nthWriteErrorWriter struct {
	buf         bytes.Buffer
	writeCount  int
	failOnWrite int
}

func (w *nthWriteErrorWriter) Write(p []byte) (int, error) {
	w.writeCount++
	if w.writeCount == w.failOnWrite {
		return 0, fmt.Errorf("nth write error")
	}
	return w.buf.Write(p)
}

// mockRecord is a test double that implements Record.
type mockRecord struct {
	id     string
	values map[string]any
}

func (m *mockRecord) GetID() string               { return m.id }
func (m *mockRecord) GetValue(name string) any     { return m.values[name] }
func (m *mockRecord) GetIntValue(_ string) int     { return 0 }
func (m *mockRecord) GetStrValue(_ string) string  { return "" }
func (m *mockRecord) GetBoolValue(_ string) bool   { return false }
func (m *mockRecord) IsCommented() bool            { return false }

// ---------------------------------------------------------------------------
// NewRecordsWriter
// ---------------------------------------------------------------------------

func TestNewRecordsWriter(t *testing.T) {
	t.Run("panic_on_nil_writer", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("NewRecordsWriter() should have panicked on nil writer")
			}
		}()
		NewRecordsWriter(nil)
	})

	t.Run("happy_path", func(t *testing.T) {
		w := &bytes.Buffer{}
		got := NewRecordsWriter(w)
		if got == nil {
			t.Fatal("NewRecordsWriter() returned unexpected nil")
		}
		if gotW := w.String(); gotW != "" {
			t.Errorf("NewRecordsWriter() gotW = %s, want empty string", gotW)
		}
	})

	t.Run("with_sha256", func(t *testing.T) {
		w := &bytes.Buffer{}
		got := NewRecordsWriter(w, SHA256)
		if got == nil {
			t.Fatal("NewRecordsWriter() with SHA256 returned nil")
		}
	})

	t.Run("panic_on_unsupported_hash", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("NewRecordsWriter() should panic on unsupported hash algorithm")
			}
		}()
		NewRecordsWriter(&bytes.Buffer{}, HashAlgorithm("md5"))
	})
}

// ---------------------------------------------------------------------------
// WriteHeader
// ---------------------------------------------------------------------------

func TestRecordsWriter_WriteHeader(t *testing.T) {
	t.Run("happy_path_no_cols", func(t *testing.T) {
		w := &bytes.Buffer{}
		rw := &recordsWriter{w: w}
		n, err := rw.WriteHeader("TestTitle")
		if err != nil {
			t.Fatalf("WriteHeader() error = %v", err)
		}
		if n == 0 {
			t.Error("WriteHeader() wrote 0 bytes")
		}
		if !strings.Contains(w.String(), "# INGR.io | TestTitle : ") {
			t.Errorf("WriteHeader() output %q missing expected header", w.String())
		}
		if !rw.headerWritten {
			t.Error("WriteHeader() should set headerWritten = true")
		}
	})

	t.Run("happy_path_with_cols", func(t *testing.T) {
		w := &bytes.Buffer{}
		rw := &recordsWriter{w: w, cols: []ColDef{{Name: "id"}, {Name: "name"}}}
		n, err := rw.WriteHeader("TestTitle")
		if err != nil {
			t.Fatalf("WriteHeader() error = %v", err)
		}
		if n == 0 {
			t.Error("WriteHeader() wrote 0 bytes")
		}
		out := w.String()
		if !strings.Contains(out, "id") || !strings.Contains(out, "name") {
			t.Errorf("WriteHeader() output %q missing column names", out)
		}
	})

	t.Run("header_already_written", func(t *testing.T) {
		rw := &recordsWriter{w: &bytes.Buffer{}, headerWritten: true}
		_, err := rw.WriteHeader("Test")
		if err == nil {
			t.Error("WriteHeader() should return error when header already written")
		}
		if err.Error() != "header already written" {
			t.Errorf("WriteHeader() error = %q, want %q", err.Error(), "header already written")
		}
	})

	t.Run("write_error", func(t *testing.T) {
		rw := &recordsWriter{w: &errorWriter{}}
		_, err := rw.WriteHeader("Test")
		if err == nil {
			t.Error("WriteHeader() should return error when writer fails")
		}
		if !strings.Contains(err.Error(), "failed to write header") {
			t.Errorf("WriteHeader() error = %q, want to contain 'failed to write header'", err.Error())
		}
	})

	t.Run("partial_write_sets_header_written", func(t *testing.T) {
		// partialErrorWriter returns n=1 and an error on the first Write,
		// which exercises the "if n > 0 { rw.headerWritten = true }" branch.
		rw := &recordsWriter{w: &partialErrorWriter{}}
		_, err := rw.WriteHeader("Test")
		if err == nil {
			t.Error("WriteHeader() should return error on partial write failure")
		}
		if !rw.headerWritten {
			t.Error("WriteHeader() should set headerWritten=true when n > 0 even on error")
		}
	})

	t.Run("column_write_error", func(t *testing.T) {
		// Fail on the 2nd Write call (the first column name).
		rw := &recordsWriter{
			w:    &nthWriteErrorWriter{failOnWrite: 2},
			cols: []ColDef{{Name: "id"}},
		}
		_, err := rw.WriteHeader("Test")
		if err == nil {
			t.Error("WriteHeader() should return error when column write fails")
		}
		if !strings.Contains(err.Error(), "failed to write column name at index 0") {
			t.Errorf("WriteHeader() error = %q, want column error", err.Error())
		}
	})
}

// ---------------------------------------------------------------------------
// WriteRecords
// ---------------------------------------------------------------------------

func TestRecordsWriter_WriteRecords(t *testing.T) {
	t.Run("no_records", func(t *testing.T) {
		w := &bytes.Buffer{}
		rw := &recordsWriter{w: w, headerWritten: true}
		n, err := rw.WriteRecords(0)
		if err != nil {
			t.Fatalf("WriteRecords() error = %v", err)
		}
		if n != 0 {
			t.Errorf("WriteRecords() n = %d, want 0", n)
		}
	})

	t.Run("with_records_no_delimiter", func(t *testing.T) {
		w := &bytes.Buffer{}
		rw := &recordsWriter{
			w:             w,
			headerWritten: true,
			cols:          []ColDef{{Name: "name"}},
		}
		r := &mockRecord{id: "1", values: map[string]any{"name": "Alice"}}
		n, err := rw.WriteRecords(0, r)
		if err != nil {
			t.Fatalf("WriteRecords() error = %v", err)
		}
		if n == 0 {
			t.Error("WriteRecords() wrote 0 bytes")
		}
		if !strings.Contains(w.String(), "Alice") {
			t.Errorf("WriteRecords() output %q missing expected value", w.String())
		}
	})

	t.Run("with_delimiter", func(t *testing.T) {
		w := &bytes.Buffer{}
		rw := &recordsWriter{
			w:             w,
			headerWritten: true,
			cols:          []ColDef{},
		}
		r := &mockRecord{id: "1"}
		n, err := rw.WriteRecords(5, r)
		if err != nil {
			t.Fatalf("WriteRecords() error = %v", err)
		}
		if n == 0 {
			t.Error("WriteRecords() wrote 0 bytes")
		}
		if !strings.Contains(w.String(), "#-----") {
			t.Errorf("WriteRecords() output %q missing delimiter", w.String())
		}
	})

	t.Run("column_write_error", func(t *testing.T) {
		rw := &recordsWriter{
			w:             &errorWriter{},
			headerWritten: true,
			cols:          []ColDef{{Name: "name"}},
		}
		r := &mockRecord{id: "1", values: map[string]any{"name": "Alice"}}
		_, err := rw.WriteRecords(0, r)
		if err == nil {
			t.Error("WriteRecords() should return error when column write fails")
		}
		if !strings.Contains(err.Error(), "failed to write column name name") {
			t.Errorf("WriteRecords() error = %q, want column error", err.Error())
		}
	})

	t.Run("delimiter_write_error", func(t *testing.T) {
		// No columns so the column loop is skipped; the delimiter write is the first Write call.
		rw := &recordsWriter{
			w:             &errorWriter{},
			headerWritten: true,
			cols:          []ColDef{},
		}
		r := &mockRecord{id: "1"}
		_, err := rw.WriteRecords(5, r)
		if err == nil {
			t.Error("WriteRecords() should return error when delimiter write fails")
		}
		if !strings.Contains(err.Error(), "failed to write records delimiter") {
			t.Errorf("WriteRecords() error = %q, want delimiter error", err.Error())
		}
	})
}

// ---------------------------------------------------------------------------
// writeFooter / Close
// ---------------------------------------------------------------------------

func TestRecordsWriter_Close(t *testing.T) {
	t.Run("without_hash", func(t *testing.T) {
		w := &bytes.Buffer{}
		rw := &recordsWriter{w: w, recordsCount: 3}
		if err := rw.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
		if !strings.Contains(w.String(), "# 3 record(s)") {
			t.Errorf("Close() output %q missing record count", w.String())
		}
	})

	t.Run("with_hash", func(t *testing.T) {
		w := &bytes.Buffer{}
		// Use NewRecordsWriter to initialise the sha256 hash properly.
		rw := NewRecordsWriter(w, SHA256).(*recordsWriter)
		if err := rw.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
		if !strings.Contains(w.String(), "# sha256:") {
			t.Errorf("Close() output %q missing hash footer", w.String())
		}
	})

	t.Run("footer_write_error", func(t *testing.T) {
		rw := &recordsWriter{w: &errorWriter{}}
		err := rw.Close()
		if err == nil {
			t.Error("Close() should return error when write fails")
		}
		if !strings.Contains(err.Error(), "failed to write records count") {
			t.Errorf("Close() error = %q, want 'failed to write records count'", err.Error())
		}
	})

	t.Run("hash_write_error", func(t *testing.T) {
		// The record-count line is the 1st Write; the hash line is the 2nd.
		// Failing on the 2nd write exercises the error-return from the hash write.
		ew := &nthWriteErrorWriter{failOnWrite: 2}
		rw := NewRecordsWriter(ew, SHA256).(*recordsWriter)
		err := rw.Close()
		if err == nil {
			t.Error("Close() should return error when hash write fails")
		}
	})
}
