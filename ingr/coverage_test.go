package ingr

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

type failReader struct{}

func (failReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("simulated read error")
}

type failAfterHeaderReader struct {
	readHeader bool
}

func (r *failAfterHeaderReader) Read(p []byte) (n int, err error) {
	if !r.readHeader {
		r.readHeader = true
		header := "# INGR.io | t: $ID\n"
		copy(p, header)
		return len(header), nil
	}
	return 0, errors.New("read error after header")
}

type errWriter struct {
	failAfter int
	count     int
}

func (e *errWriter) Write(p []byte) (n int, err error) {
	e.count++
	if e.count > e.failAfter {
		return 0, errors.New("simulated write error")
	}
	return len(p), nil
}

func TestErrors(t *testing.T) {
	syn := &SyntaxError{Line: 3, Offset: 10, msg: "bad syntax"}
	if got := syn.Error(); got != "ingr: bad syntax (line 3, offset 10)" {
		t.Errorf("SyntaxError.Error() = %q", got)
	}

	uteField := &UnmarshalTypeError{Value: "string", Type: reflect.TypeOf(0), Line: 2, Field: "age"}
	if got := uteField.Error(); !strings.Contains(got, "field .age") {
		t.Errorf("UnmarshalTypeError with field = %q", got)
	}

	uteNoField := &UnmarshalTypeError{Value: "string", Type: reflect.TypeOf(0), Line: 2, Field: ""}
	if got := uteNoField.Error(); !strings.Contains(got, "Go value of type int") {
		t.Errorf("UnmarshalTypeError without field = %q", got)
	}
}

func TestIngrOptions(t *testing.T) {
	var opts ExportOptions
	ApplyOptions(&opts, WithHash(), WithRecordsDelimiter())
	if !opts.IncludeHash || !opts.RecordsDelimiter {
		t.Errorf("ApplyOptions failed: %+v", opts)
	}
}

func TestRawValueCoverage(t *testing.T) {
	var rNil RawValue
	b, err := rNil.MarshalINGR()
	if err != nil || string(b) != "null" {
		t.Errorf("MarshalINGR nil = %q, %v", string(b), err)
	}

	rVal := RawValue("123")
	b, err = rVal.MarshalINGR()
	if err != nil || string(b) != "123" {
		t.Errorf("MarshalINGR val = %q, %v", string(b), err)
	}

	var rNilPtr *RawValue
	if err := rNilPtr.UnmarshalINGR([]byte("abc")); err == nil {
		t.Errorf("expected error on (*RawValue)(nil).UnmarshalINGR")
	}

	var rValid RawValue
	if err := rValid.UnmarshalINGR([]byte("abc")); err != nil || string(rValid) != "abc" {
		t.Errorf("UnmarshalINGR valid = %q, %v", string(rValid), err)
	}
}

func TestRecordEntryGetters(t *testing.T) {
	rec := NewMapRecordEntry("id-123", map[string]any{
		"s": "foo",
		"b": true,
		"i": 42,
	})
	if got := rec.GetID(); got != "id-123" {
		t.Errorf("GetID() = %q, want id-123", got)
	}
	if got := rec.GetStrValue("s"); got != "foo" {
		t.Errorf("GetStrValue(s) = %q, want foo", got)
	}
	if got := rec.GetBoolValue("b"); !got {
		t.Errorf("GetBoolValue(b) = false, want true")
	}
	if got := rec.GetValue("$ID"); got != "id-123" {
		t.Errorf("GetValue($ID) = %v, want id-123", got)
	}
}

func TestParseHeaderErrors(t *testing.T) {
	cases := []struct {
		input   string
		wantErr string
	}{
		{"INGR.io | table: id", "header must start with '#'"},
		{"# other | table: id", "header must contain 'INGR.io'"},
		{"# INGR.io table: id", "header must contain '|' after 'INGR.io'"},
		{"# INGR.io | table", "header missing ':' after recordset name"},
		{"# INGR.io | : id", "empty recordset name in header"},
		{"# INGR.io | table: , :int", "header has no columns"},
	}
	for _, tc := range cases {
		_, err := parseHeader(tc.input)
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("parseHeader(%q) err = %v, want substring %q", tc.input, err, tc.wantErr)
		}
	}
}

func TestValidateCoverage(t *testing.T) {
	// Valid with 0 records
	emptyInput := join(
		`# INGR.io | people: $ID, name`,
		`# 0 records`,
	)
	if err := Validate([]byte(emptyInput)); err != nil {
		t.Errorf("Validate emptyInput: %v", err)
	}

	// Invalid header
	if err := Validate([]byte("not a header")); err == nil {
		t.Errorf("expected Validate error on invalid header")
	}

	// Invalid record JSON
	badRecord := join(
		`# INGR.io | people: $ID, name`,
		`"1"`,
		`{bad json`,
		`# 1 record`,
	)
	if err := Validate([]byte(badRecord)); err == nil {
		t.Errorf("expected Validate error on bad record")
	}

	// Footer count mismatch
	badFooter := join(
		`# INGR.io | people: $ID, name`,
		`"1"`,
		`"Alice"`,
		`# 5 records`,
	)
	if err := Validate([]byte(badFooter)); err == nil {
		t.Errorf("expected Validate error on footer mismatch")
	}
}

func TestRecordsWriterCoverage(t *testing.T) {
	// Unsupported hash algorithm panic
	t.Run("unsupported_hash_panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("expected panic on unsupported hash")
			}
		}()
		NewRecordsWriter(&bytes.Buffer{}, "md5")
	})

	// SHA256 footer line
	t.Run("sha256_footer", func(t *testing.T) {
		buf := &bytes.Buffer{}
		rw := NewRecordsWriter(buf, SHA256)
		if _, err := rw.WriteHeader("items", []ColDef{{Name: "$ID", Type: "string"}}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if _, err := rw.WriteRecords(0, NewMapRecordEntry("1", map[string]any{})); err != nil {
			t.Fatalf("WriteRecords: %v", err)
		}
		if err := rw.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "# sha256:") {
			t.Errorf("expected sha256 in footer: %s", out)
		}
	})

	// WriteHeader write error
	t.Run("write_header_fail", func(t *testing.T) {
		ew := &errWriter{failAfter: 0}
		rw := NewRecordsWriter(ew)
		if _, err := rw.WriteHeader("t", []ColDef{{Name: "$ID"}}); err == nil {
			t.Errorf("expected error on failed header write")
		}
	})

	// WriteRecords value JSON marshal error
	t.Run("write_records_marshal_fail", func(t *testing.T) {
		rw := NewRecordsWriter(&bytes.Buffer{})
		if _, err := rw.WriteHeader("t", []ColDef{{Name: "ch"}}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		rec := NewMapRecordEntry("1", map[string]any{"ch": make(chan int)})
		if _, err := rw.WriteRecords(0, rec); err == nil {
			t.Errorf("expected error marshalling chan to JSON")
		}
	})

	// WriteRecords value write error
	t.Run("write_records_val_write_fail", func(t *testing.T) {
		ew := &errWriter{failAfter: 1} // allows header write, fails on value write
		rw := NewRecordsWriter(ew)
		if _, err := rw.WriteHeader("t", []ColDef{{Name: "$ID"}}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		rec := NewMapRecordEntry("1", map[string]any{})
		if _, err := rw.WriteRecords(0, rec); err == nil {
			t.Errorf("expected error on value write failure")
		}
	})

	// WriteRecords delimiter write error
	t.Run("write_records_delimiter_write_fail", func(t *testing.T) {
		ew := &errWriter{failAfter: 2} // header (1), value (2), delimiter fails (3)
		rw := NewRecordsWriter(ew)
		if _, err := rw.WriteHeader("t", []ColDef{{Name: "$ID"}}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		rec := NewMapRecordEntry("1", map[string]any{})
		if _, err := rw.WriteRecords(1, rec); err == nil {
			t.Errorf("expected error on delimiter write failure")
		}
	})

	// writeFooter failure without hash
	t.Run("write_footer_fail_no_hash", func(t *testing.T) {
		ew := &errWriter{failAfter: 1} // header write succeeds (1), footer fails (2)
		rw := NewRecordsWriter(ew)
		if _, err := rw.WriteHeader("t", []ColDef{{Name: "$ID"}}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if err := rw.Close(); err == nil {
			t.Errorf("expected error closing on failed footer write")
		}
	})

	// writeFooter failure with hash on count line
	t.Run("write_footer_fail_with_hash_count", func(t *testing.T) {
		ew := &errWriter{failAfter: 1} // header write succeeds (1), count line fails (2)
		rw := NewRecordsWriter(ew, SHA256)
		if _, err := rw.WriteHeader("t", []ColDef{{Name: "$ID"}}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if err := rw.Close(); err == nil {
			t.Errorf("expected error closing on failed count line write")
		}
	})

	// writeFooter failure with hash on hash line
	t.Run("write_footer_fail_with_hash_hash", func(t *testing.T) {
		ew := &errWriter{failAfter: 2} // header write succeeds (1), count line succeeds (2), hash line fails (3)
		rw := NewRecordsWriter(ew, SHA256)
		if _, err := rw.WriteHeader("t", []ColDef{{Name: "$ID"}}); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if err := rw.Close(); err == nil {
			t.Errorf("expected error closing on failed hash line write")
		}
	})
}

func TestDecoderCoverage(t *testing.T) {
	// Header() before and after ReadHeader
	t.Run("header_method", func(t *testing.T) {
		input := "# INGR.io | people: $ID, name\n\"1\"\n\"Alice\"\n# 1 record"
		dec := NewDecoder(strings.NewReader(input))
		if h := dec.Header(); h.Recordset != "" {
			t.Errorf("expected empty header before read, got %+v", h)
		}
		h, err := dec.ReadHeader()
		if err != nil {
			t.Fatalf("ReadHeader: %v", err)
		}
		if got := dec.Header().Recordset; got != "people" {
			t.Errorf("Header().Recordset = %q, want people", got)
		}
		// Second ReadHeader returns cached header
		h2, err := dec.ReadHeader()
		if err != nil || h2.Recordset != h.Recordset {
			t.Errorf("cached ReadHeader failed: %v", err)
		}
	})

	// ReadHeader on empty stream
	t.Run("read_header_empty", func(t *testing.T) {
		dec := NewDecoder(strings.NewReader(""))
		if _, err := dec.ReadHeader(); err == nil {
			t.Errorf("expected error reading header from empty stream")
		}
	})

	// More() and Decode() on error and EOF
	t.Run("more_and_decode_edge_cases", func(t *testing.T) {
		dec := NewDecoder(failReader{})
		if dec.More() {
			t.Errorf("expected More() = false on read error")
		}
		// dec.err is now set, calling More() again tests `if d.done || d.err != nil { return false }`
		if dec.More() {
			t.Errorf("expected More() = false when err is set")
		}
		if err := dec.Decode(nil); err == nil {
			t.Errorf("expected error from Decode on failed reader")
		}

		// sticky err in Decode
		if err := dec.Decode(nil); err == nil {
			t.Errorf("expected sticky error on subsequent Decode")
		}

		// Decode when done
		doneDec := NewDecoder(strings.NewReader("# INGR.io | t: $ID\n# 0 records"))
		var m map[string]any
		if err := doneDec.Decode(&m); err != io.EOF {
			t.Errorf("expected io.EOF on 0 records stream, got %v", err)
		}
		// Calling More() when done is true tests `if d.done || d.err != nil { return false }`
		if doneDec.More() {
			t.Errorf("expected More() = false when done")
		}
		// Subsequent Decode when done returns io.EOF
		if err := doneDec.Decode(&m); err != io.EOF {
			t.Errorf("expected io.EOF when done, got %v", err)
		}

		// Decode directly on empty stream without calling More first (tests ensureHeader in Decode)
		emptyDec := NewDecoder(strings.NewReader(""))
		if err := emptyDec.Decode(&m); err == nil {
			t.Errorf("expected error from Decode on empty stream")
		}

		// Header followed immediately by EOF (tests peekLine error in More)
		headerOnlyDec := NewDecoder(strings.NewReader("# INGR.io | t: $ID\n"))
		if headerOnlyDec.More() {
			t.Errorf("expected More() = false on header-only stream")
		}

		// peekLine error with scanner.Err
		failAfterHeader := NewDecoder(&failAfterHeaderReader{})
		if failAfterHeader.More() {
			t.Errorf("expected More() = false when reader fails after header")
		}
	})

	// Decode(nil) skips a record
	t.Run("decode_nil_skips", func(t *testing.T) {
		input := join(
			`# INGR.io | people: $ID, name`,
			`"1"`,
			`"Alice"`,
			`"2"`,
			`"Bob"`,
			`# 2 records`,
		)
		dec := NewDecoder(strings.NewReader(input))
		if err := dec.Decode(nil); err != nil {
			t.Fatalf("Decode(nil): %v", err)
		}
		var p person
		if err := dec.Decode(&p); err != nil || p.Name != "Bob" {
			t.Fatalf("Decode second record: %v, %+v", err, p)
		}
	})

	// Decode invalid targets
	t.Run("decode_invalid_targets", func(t *testing.T) {
		makeDec := func() *Decoder {
			return NewDecoder(strings.NewReader("# INGR.io | t: $ID\n\"1\"\n# 1 record"))
		}
		if err := makeDec().Decode(123); err == nil {
			t.Errorf("expected error on non-pointer")
		}
		if err := makeDec().Decode((*person)(nil)); err == nil {
			t.Errorf("expected error on nil pointer")
		}
		n := 42
		if err := makeDec().Decode(&n); err == nil || !strings.Contains(err.Error(), "unsupported decode target type") {
			t.Errorf("expected error on unsupported pointer type *int, got %v", err)
		}
	})

	// decodeAll with error during record decoding and with footer mismatch
	t.Run("decode_all_errors", func(t *testing.T) {
		badStream := join(
			`# INGR.io | people: $ID, name`,
			`"1"`,
			`{bad json`,
			`# 1 record`,
		)
		dec := NewDecoder(strings.NewReader(badStream))
		var all []map[string]any
		if err := dec.Decode(&all); err == nil {
			t.Errorf("expected error decoding all into slice on bad record")
		}

		// Footer count mismatch during decodeAll
		mismatchedFooter := join(
			`# INGR.io | people: $ID`,
			`"1"`,
			`# 5 records`,
		)
		decMismatched := NewDecoder(strings.NewReader(mismatchedFooter))
		var all2 []map[string]any
		if err := decMismatched.Decode(&all2); err == nil {
			t.Errorf("expected error from decodeAll on mismatched footer count")
		}
	})

	// Short record (fewer fields than columns in header)
	t.Run("short_record", func(t *testing.T) {
		shortRec := join(
			`# INGR.io | people: $ID, name, age`,
			`"1"`,
			`"Alice"`,
		)
		dec := NewDecoder(strings.NewReader(shortRec))
		var m map[string]any
		if err := dec.Decode(&m); err == nil {
			t.Errorf("expected error on short record")
		}
	})

	// Struct decode: unexported fields, tag with empty name, ingr:"-"
	t.Run("struct_field_handling", func(t *testing.T) {
		type specialStruct struct {
			unexported  string
			Named       string `ingr:"named"`
			TaggedEmpty string `ingr:",omitempty"`
			Skipped     string `ingr:"-"`
			NoTag       string
		}
		input := join(
			`# INGR.io | t: named, taggedempty, unexported, notag`,
			`"hello"`,
			`"world"`,
			`"secret"`,
			`"direct"`,
			`# 1 record`,
		)
		dec := NewDecoder(strings.NewReader(input))
		var s specialStruct
		if err := dec.Decode(&s); err != nil {
			t.Fatalf("Decode special struct: %v", err)
		}
		if s.Named != "hello" || s.TaggedEmpty != "world" || s.NoTag != "direct" || s.unexported != "" {
			t.Errorf("unexpected fields: %+v", s)
		}
	})

	// Struct with Number field type mismatch
	t.Run("struct_number_mismatch", func(t *testing.T) {
		type structWithNumber struct {
			ID  string `ingr:"$ID"`
			Num Number `ingr:"num"`
		}
		badNum := join(
			`# INGR.io | t: $ID, num`,
			`"1"`,
			`"not-a-number"`,
			`# 1 record`,
		)
		dec := NewDecoder(strings.NewReader(badNum))
		var sn structWithNumber
		if err := dec.Decode(&sn); err == nil {
			t.Errorf("expected UnmarshalTypeError for non-number into Number")
		}
	})

	// Struct with any / interface{} field: invalid JSON and null value
	t.Run("struct_any_field", func(t *testing.T) {
		type structWithAny struct {
			ID  string `ingr:"$ID"`
			Val any    `ingr:"val"`
		}
		// Invalid JSON in interface field
		badAny := join(
			`# INGR.io | t: $ID, val`,
			`"1"`,
			`{bad`,
			`# 1 record`,
		)
		dec := NewDecoder(strings.NewReader(badAny))
		var sa structWithAny
		if err := dec.Decode(&sa); err == nil {
			t.Errorf("expected SyntaxError for bad JSON in interface field")
		}

		// null in interface field
		nullAny := join(
			`# INGR.io | t: $ID, val`,
			`"1"`,
			`null`,
			`# 1 record`,
		)
		dec = NewDecoder(strings.NewReader(nullAny))
		if err := dec.Decode(&sa); err != nil || sa.Val != nil {
			t.Errorf("null interface field err = %v, val = %v", err, sa.Val)
		}

		// non-null in interface field
		validAny := join(
			`# INGR.io | t: $ID, val`,
			`"1"`,
			`"hello"`,
			`# 1 record`,
		)
		dec = NewDecoder(strings.NewReader(validAny))
		if err := dec.Decode(&sa); err != nil || sa.Val != "hello" {
			t.Errorf("valid interface field err = %v, val = %v", err, sa.Val)
		}
	})

	// Struct concrete field type mismatch
	t.Run("struct_type_mismatch", func(t *testing.T) {
		type structWithInt struct {
			ID    string `ingr:"$ID"`
			Count int    `ingr:"count"`
		}
		typeMismatch := join(
			`# INGR.io | t: $ID, count`,
			`"1"`,
			`"hello"`,
			`# 1 record`,
		)
		dec := NewDecoder(strings.NewReader(typeMismatch))
		var si structWithInt
		if err := dec.Decode(&si); err == nil {
			t.Errorf("expected UnmarshalTypeError for string into int")
		}
	})

	// convertNumbers with nested map and slice
	t.Run("convert_numbers_nested", func(t *testing.T) {
		input := join(
			`# INGR.io | t: $ID, meta, list`,
			`"1"`,
			`{"count": 42}`,
			`[10, 20]`,
			`# 1 record`,
		)
		dec := NewDecoder(strings.NewReader(input))
		dec.UseNumber()
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			t.Fatalf("Decode UseNumber nested: %v", err)
		}
		meta := m["meta"].(map[string]any)
		if meta["count"] != Number("42") {
			t.Errorf("expected Number(42), got %T: %v", meta["count"], meta["count"])
		}
		list := m["list"].([]any)
		if list[0] != Number("10") || list[1] != Number("20") {
			t.Errorf("expected Number(10), Number(20), got %v", list)
		}
	})

	// readFooter edge cases: stream ends before footer, trailing non-# line, direct calls
	t.Run("read_footer_edge_cases", func(t *testing.T) {
		// No footer at end of stream
		noFooter := join(
			`# INGR.io | t: $ID`,
			`"1"`,
		)
		dec := NewDecoder(strings.NewReader(noFooter))
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			t.Fatalf("Decode without footer: %v", err)
		}
		if err := dec.Decode(&m); err != io.EOF {
			t.Errorf("expected io.EOF, got %v", err)
		}

		// Footer with trailing non-# line
		footerWithExtra := join(
			`# INGR.io | t: $ID`,
			`"1"`,
			`# 1 record`,
			`non-comment-line`,
		)
		dec = NewDecoder(strings.NewReader(footerWithExtra))
		if err := dec.Decode(&m); err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if err := dec.Decode(&m); err != io.EOF {
			t.Errorf("expected EOF, got %v", err)
		}

		// Direct calls to readFooter when EOF or not a footer count line
		decEOF := NewDecoder(strings.NewReader(""))
		if err := decEOF.readFooter(); err != nil {
			t.Errorf("readFooter EOF: %v", err)
		}
		decNotFooter := NewDecoder(strings.NewReader("not footer line\n"))
		if err := decNotFooter.readFooter(); err != nil {
			t.Errorf("readFooter not footer: %v", err)
		}

		// parseFooterCount on invalid format
		if n := parseFooterCount("invalid footer line"); n != -1 {
			t.Errorf("parseFooterCount('invalid') = %d, want -1", n)
		}
	})

	// stripCommentPrefix edge cases
	t.Run("strip_comment_prefix", func(t *testing.T) {
		if got := stripCommentPrefix("no-hash"); got != "no-hash" {
			t.Errorf("stripCommentPrefix('no-hash') = %q", got)
		}
		if got := stripCommentPrefix(""); got != "" {
			t.Errorf("stripCommentPrefix('') = %q", got)
		}
	})
}
