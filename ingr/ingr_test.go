package ingr

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// join builds a complete INGR file from lines with '\n' separators.
// The final line has NO trailing newline (per spec).
func join(lines ...string) string {
	return strings.Join(lines, "\n")
}

// joinNL is like join but adds a trailing newline — used to test the
// "parsers must accept with or without trailing newline" requirement.
func joinNL(lines ...string) string {
	return strings.Join(lines, "\n") + "\n"
}

// ---------------------------------------------------------------------------
// Test structs
// ---------------------------------------------------------------------------

type person struct {
	ID   string `ingr:"$ID"`
	Name string `ingr:"name"`
	Age  int    `ingr:"age"`
}

type personWithRaw struct {
	ID      string   `ingr:"$ID"`
	Kind    string   `ingr:"kind"`
	Payload RawValue `ingr:"payload"`
}

type personPartial struct {
	ID   string `ingr:"$ID"`
	Name string `ingr:"name"`
	// Age is intentionally missing — tests DisallowUnknownFields.
}

type personWithSkip struct {
	ID      string `ingr:"$ID"`
	Name    string `ingr:"name"`
	Ignored string `ingr:"-"`
}

// ---------------------------------------------------------------------------
// 1. Basic decode into struct with ingr tags
// ---------------------------------------------------------------------------

func TestDecodeStruct(t *testing.T) {
	input := join(
		`# INGR.io | people: $ID, name, age`,
		`"1"`,
		`"Alice"`,
		`30`,
		`# 1 record`,
	)
	dec := NewDecoder(strings.NewReader(input))
	var p person
	if err := dec.Decode(&p); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if p.ID != "1" || p.Name != "Alice" || p.Age != 30 {
		t.Fatalf("unexpected: %+v", p)
	}
}

// ---------------------------------------------------------------------------
// 2. Decode into map[string]any
// ---------------------------------------------------------------------------

func TestDecodeMap(t *testing.T) {
	input := join(
		`# INGR.io | people: $ID, name, age`,
		`"1"`,
		`"Alice"`,
		`30`,
		`# 1 record`,
	)
	dec := NewDecoder(strings.NewReader(input))
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if m["$ID"] != "1" {
		t.Fatalf("$ID = %v, want \"1\"", m["$ID"])
	}
	if m["name"] != "Alice" {
		t.Fatalf("name = %v, want \"Alice\"", m["name"])
	}
	if m["age"] != float64(30) {
		t.Fatalf("age = %v (%T), want float64(30)", m["age"], m["age"])
	}
}

// ---------------------------------------------------------------------------
// 3. Decode into []map[string]any (all records)
// ---------------------------------------------------------------------------

func TestDecodeSliceAll(t *testing.T) {
	input := join(
		`# INGR.io | people: $ID, name`,
		`"1"`,
		`"Alice"`,
		`"2"`,
		`"Bob"`,
		`# 2 records`,
	)
	var records []map[string]any
	if err := Unmarshal([]byte(input), &records); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}
	if records[0]["name"] != "Alice" {
		t.Fatalf("records[0][name] = %v", records[0]["name"])
	}
	if records[1]["name"] != "Bob" {
		t.Fatalf("records[1][name] = %v", records[1]["name"])
	}
}

// ---------------------------------------------------------------------------
// 4. Header with type annotations
// ---------------------------------------------------------------------------

func TestHeaderTypeAnnotations(t *testing.T) {
	input := join(
		`# INGR.io | items: $ID:string, count:int, price:decimal`,
		`"x1"`,
		`10`,
		`1.99`,
		`# 1 record`,
	)
	dec := NewDecoder(strings.NewReader(input))
	h, err := dec.ReadHeader()
	if err != nil {
		t.Fatalf("ReadHeader: %v", err)
	}
	if len(h.Columns) != 3 {
		t.Fatalf("columns = %d, want 3", len(h.Columns))
	}
	tests := []struct {
		name, typ string
	}{
		{"$ID", "string"},
		{"count", "int"},
		{"price", "decimal"},
	}
	for i, tt := range tests {
		if h.Columns[i].Name != tt.name || h.Columns[i].Type != tt.typ {
			t.Errorf("col[%d] = %+v, want {%s %s}", i, h.Columns[i], tt.name, tt.typ)
		}
	}
}

// ---------------------------------------------------------------------------
// 5. File with record delimiters (#-)
// ---------------------------------------------------------------------------

func TestDelimiters(t *testing.T) {
	input := join(
		`# INGR.io | people: $ID, name`,
		`"1"`,
		`"Alice"`,
		`#-----`,
		`"2"`,
		`"Bob"`,
		`#-----`,
		`# 2 records`,
	)
	var records []map[string]any
	if err := Unmarshal([]byte(input), &records); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}
}

// ---------------------------------------------------------------------------
// 6. Commented-out record (full)
// ---------------------------------------------------------------------------

func TestCommentedOutRecord(t *testing.T) {
	input := join(
		`# INGR.io | people: $ID, name`,
		`#"1"`,
		`#"Alice"`,
		`"2"`,
		`"Bob"`,
		`# 2 records`,
	)
	dec := NewDecoder(strings.NewReader(input))

	// First record is commented-out — still decoded (skipped values).
	var m1 map[string]any
	if err := dec.Decode(&m1); err != nil {
		t.Fatalf("Decode record 1: %v", err)
	}
	if m1["$ID"] != "1" {
		t.Fatalf("record 1 $ID = %v, want \"1\"", m1["$ID"])
	}

	// Second record is normal.
	var m2 map[string]any
	if err := dec.Decode(&m2); err != nil {
		t.Fatalf("Decode record 2: %v", err)
	}
	if m2["name"] != "Bob" {
		t.Fatalf("record 2 name = %v", m2["name"])
	}
}

// ---------------------------------------------------------------------------
// 7. Partial commented-out record → error
// ---------------------------------------------------------------------------

func TestPartialCommentError(t *testing.T) {
	input := join(
		`# INGR.io | people: $ID, name`,
		`#"1"`,
		`"Alice"`,
		`# 1 record`,
	)
	dec := NewDecoder(strings.NewReader(input))
	var m map[string]any
	err := dec.Decode(&m)
	if err == nil {
		t.Fatal("expected error for partial comment, got nil")
	}
	var se *SyntaxError
	if !errors.As(err, &se) {
		t.Fatalf("expected *SyntaxError, got %T: %v", err, err)
	}
	if !strings.Contains(se.Error(), "partially commented-out") {
		t.Fatalf("unexpected error message: %v", se)
	}
}

// ---------------------------------------------------------------------------
// 8. Inline comments on value lines
// ---------------------------------------------------------------------------

func TestInlineComments(t *testing.T) {
	input := join(
		`# INGR.io | people: $ID, name, active`,
		`"1"`,
		`"Alice"`,
		`true # was set by admin`,
		`# 1 record`,
	)
	dec := NewDecoder(strings.NewReader(input))
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if m["active"] != true {
		t.Fatalf("active = %v (%T), want true", m["active"], m["active"])
	}
}

// ---------------------------------------------------------------------------
// 9. Inline comment inside a JSON string (must NOT strip)
// ---------------------------------------------------------------------------

func TestInlineCommentInsideString(t *testing.T) {
	input := join(
		`# INGR.io | people: $ID, name`,
		`"1"`,
		`"Alice # in Wonderland"`,
		`# 1 record`,
	)
	dec := NewDecoder(strings.NewReader(input))
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if m["name"] != "Alice # in Wonderland" {
		t.Fatalf("name = %q, want %q", m["name"], "Alice # in Wonderland")
	}
}

// ---------------------------------------------------------------------------
// 10. Footer record count mismatch → error
// ---------------------------------------------------------------------------

func TestFooterCountMismatch(t *testing.T) {
	input := join(
		`# INGR.io | people: $ID, name`,
		`"1"`,
		`"Alice"`,
		`# 5 records`,
	)
	dec := NewDecoder(strings.NewReader(input))
	var m map[string]any

	// First Decode succeeds (reads one record).
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	// Second Decode hits footer with count 5 vs 1 actual.
	err := dec.Decode(&m)
	if err == nil {
		t.Fatal("expected footer mismatch error, got nil")
	}
	var se *SyntaxError
	if !errors.As(err, &se) {
		t.Fatalf("expected *SyntaxError, got %T: %v", err, err)
	}
	if !strings.Contains(se.Error(), "footer says 5 records but found 1") {
		t.Fatalf("unexpected error: %v", se)
	}
}

// ---------------------------------------------------------------------------
// 11. UseNumber preserves large integers as Number
// ---------------------------------------------------------------------------

func TestUseNumber(t *testing.T) {
	input := join(
		`# INGR.io | data: $ID, big_num`,
		`"1"`,
		`9223372036854775807`,
		`# 1 record`,
	)
	dec := NewDecoder(strings.NewReader(input))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	n, ok := m["big_num"].(Number)
	if !ok {
		t.Fatalf("big_num type = %T, want ingr.Number", m["big_num"])
	}
	got, err := n.Int64()
	if err != nil {
		t.Fatalf("Int64: %v", err)
	}
	if got != 9223372036854775807 {
		t.Fatalf("got %d, want 9223372036854775807", got)
	}
}

// ---------------------------------------------------------------------------
// 12. DisallowUnknownFields → error on unknown column
// ---------------------------------------------------------------------------

func TestDisallowUnknownFields(t *testing.T) {
	input := join(
		`# INGR.io | people: $ID, name, age`,
		`"1"`,
		`"Alice"`,
		`30`,
		`# 1 record`,
	)
	dec := NewDecoder(strings.NewReader(input))
	dec.DisallowUnknownFields()

	// personPartial has no "age" field.
	var p personPartial
	err := dec.Decode(&p)
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
	if !strings.Contains(err.Error(), `unknown field "age"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 13. RawValue deferred parsing
// ---------------------------------------------------------------------------

func TestRawValue(t *testing.T) {
	input := join(
		`# INGR.io | events: $ID, kind, payload`,
		`"1"`,
		`"click"`,
		`{"x":100,"y":200}`,
		`# 1 record`,
	)
	dec := NewDecoder(strings.NewReader(input))
	var ev personWithRaw
	if err := dec.Decode(&ev); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if ev.ID != "1" || ev.Kind != "click" {
		t.Fatalf("unexpected: %+v", ev)
	}
	if string(ev.Payload) != `{"x":100,"y":200}` {
		t.Fatalf("payload = %q", string(ev.Payload))
	}
}

// ---------------------------------------------------------------------------
// 14. Validate() on valid and invalid inputs
// ---------------------------------------------------------------------------

func TestValidate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		input := join(
			`# INGR.io | things: $ID, val`,
			`"1"`,
			`42`,
			`# 1 record`,
		)
		if err := Validate([]byte(input)); err != nil {
			t.Fatalf("Validate: %v", err)
		}
	})

	t.Run("invalid_header", func(t *testing.T) {
		input := "not a valid header\n\"1\"\n# 1 record"
		if err := Validate([]byte(input)); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("invalid_json_value", func(t *testing.T) {
		input := join(
			`# INGR.io | things: $ID, val`,
			`"1"`,
			`{bad json}`,
			`# 1 record`,
		)
		if err := Validate([]byte(input)); err == nil {
			t.Fatal("expected error for bad JSON, got nil")
		}
	})

	t.Run("footer_count_mismatch", func(t *testing.T) {
		input := join(
			`# INGR.io | things: $ID`,
			`"1"`,
			`# 99 records`,
		)
		err := Validate([]byte(input))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "footer says 99 records but found 1") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// 15. Header parsing: pipe is mandatory
// ---------------------------------------------------------------------------

func TestHeaderParsing(t *testing.T) {
	t.Run("with_pipe", func(t *testing.T) {
		h, err := parseHeader(`# INGR.io | people: $ID, name`)
		if err != nil {
			t.Fatalf("parseHeader: %v", err)
		}
		if h.Recordset != "people" {
			t.Errorf("recordset = %q, want %q", h.Recordset, "people")
		}
		if len(h.Columns) != 2 {
			t.Errorf("columns = %d, want 2", len(h.Columns))
		}
	})

	t.Run("without_pipe_is_error", func(t *testing.T) {
		_, err := parseHeader(`# INGR.io people: $ID, name`)
		if err == nil {
			t.Fatal("expected error for missing '|', got nil")
		}
	})

	t.Run("extra_spaces", func(t *testing.T) {
		h, err := parseHeader(`#  INGR.io  |  items  :  $ID  ,  price:decimal  `)
		if err != nil {
			t.Fatalf("parseHeader: %v", err)
		}
		if h.Recordset != "items" {
			t.Errorf("recordset = %q, want %q", h.Recordset, "items")
		}
		if len(h.Columns) != 2 {
			t.Errorf("columns = %d, want 2", len(h.Columns))
		}
	})
}

// ---------------------------------------------------------------------------
// 16. More() usage pattern
// ---------------------------------------------------------------------------

func TestMorePattern(t *testing.T) {
	input := join(
		`# INGR.io | data: $ID, v`,
		`"a"`,
		`1`,
		`"b"`,
		`2`,
		`# 2 records`,
	)
	dec := NewDecoder(strings.NewReader(input))
	var results []map[string]any
	for dec.More() {
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			t.Fatalf("Decode: %v", err)
		}
		results = append(results, m)
	}
	if len(results) != 2 {
		t.Fatalf("got %d records, want 2", len(results))
	}
}

// ---------------------------------------------------------------------------
// 17. More() stops when file is exhausted
// ---------------------------------------------------------------------------

func TestMoreStopsAtEOF(t *testing.T) {
	// A minimal file with 0 records.
	input := join(
		`# INGR.io | empty: $ID`,
		`# 0 records`,
	)
	dec := NewDecoder(strings.NewReader(input))
	if dec.More() {
		t.Fatal("More() should return false for 0-record file")
	}
}

// ---------------------------------------------------------------------------
// 18. Header | - with no trailing newline in input
// ---------------------------------------------------------------------------

func TestNoTrailingNewline(t *testing.T) {
	// File content with NO trailing newline after the footer.
	input := join(
		`# INGR.io | things: $ID, val`,
		`"x"`,
		`99`,
		`# 1 record`,
	)
	// Verify there is no trailing newline.
	if strings.HasSuffix(input, "\n") {
		t.Fatal("test bug: input should NOT have trailing newline")
	}

	dec := NewDecoder(strings.NewReader(input))
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if m["val"] != float64(99) {
		t.Fatalf("val = %v, want 99", m["val"])
	}

	// With trailing newline — must also work.
	input2 := input + "\n"
	dec2 := NewDecoder(strings.NewReader(input2))
	var m2 map[string]any
	if err := dec2.Decode(&m2); err != nil {
		t.Fatalf("Decode (trailing NL): %v", err)
	}
	if m2["val"] != float64(99) {
		t.Fatalf("val = %v, want 99", m2["val"])
	}
}

// ---------------------------------------------------------------------------
// Additional coverage
// ---------------------------------------------------------------------------

func TestUnmarshalString(t *testing.T) {
	input := join(
		`# INGR.io | x: $ID`,
		`"hello"`,
		`# 1 record`,
	)
	var m map[string]any
	if err := UnmarshalString(input, &m); err != nil {
		t.Fatalf("UnmarshalString: %v", err)
	}
	if m["$ID"] != "hello" {
		t.Fatalf("$ID = %v", m["$ID"])
	}
}

func TestHeaderString(t *testing.T) {
	h := Header{
		Recordset: "items",
		Columns: []Column{
			{Name: "$ID", Type: "string"},
			{Name: "price", Type: "decimal"},
			{Name: "tags"},
		},
	}
	want := "# INGR.io | items: $ID:string, price:decimal, tags"
	got := h.String()
	if got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestNumberMethods(t *testing.T) {
	n := Number("42")
	if n.String() != "42" {
		t.Errorf("String() = %q", n.String())
	}
	f, err := n.Float64()
	if err != nil || f != 42.0 {
		t.Errorf("Float64() = %v, %v", f, err)
	}
	i, err := n.Int64()
	if err != nil || i != 42 {
		t.Errorf("Int64() = %v, %v", i, err)
	}
}

func TestRawValueMarshal(t *testing.T) {
	r := RawValue(`{"a":1}`)
	b, err := r.MarshalINGR()
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"a":1}` {
		t.Fatalf("MarshalINGR = %q", string(b))
	}

	var nilR RawValue
	b2, _ := nilR.MarshalINGR()
	if string(b2) != "null" {
		t.Fatalf("nil MarshalINGR = %q", string(b2))
	}
}

func TestParseTag(t *testing.T) {
	tests := []struct {
		tag  string
		want fieldTag
	}{
		{"-", fieldTag{skip: true}},
		{"name", fieldTag{name: "name"}},
		{"name,omitempty", fieldTag{name: "name", omitempty: true}},
		{"$ID", fieldTag{name: "$ID"}},
	}
	for _, tt := range tests {
		got := parseTag(tt.tag)
		if got != tt.want {
			t.Errorf("parseTag(%q) = %+v, want %+v", tt.tag, got, tt.want)
		}
	}
}

func TestDecodeEOFOnDone(t *testing.T) {
	input := join(
		`# INGR.io | x: $ID`,
		`"1"`,
		`# 1 record`,
	)
	dec := NewDecoder(strings.NewReader(input))
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	// Second call should return io.EOF.
	err := dec.Decode(&m)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF, got %v", err)
	}
}

func TestDecodeSkipNil(t *testing.T) {
	input := join(
		`# INGR.io | x: $ID, v`,
		`"1"`,
		`10`,
		`"2"`,
		`20`,
		`# 2 records`,
	)
	dec := NewDecoder(strings.NewReader(input))
	// Skip first record.
	if err := dec.Decode(nil); err != nil {
		t.Fatalf("Decode(nil): %v", err)
	}
	// Read second.
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if m["$ID"] != "2" {
		t.Fatalf("$ID = %v, want \"2\"", m["$ID"])
	}
}

func TestStructFallbackLowercase(t *testing.T) {
	// Struct without ingr tags — fields matched by lowercased name.
	type simple struct {
		Name string
		Age  int
	}
	input := join(
		`# INGR.io | data: name, age`,
		`"Eve"`,
		`28`,
		`# 1 record`,
	)
	dec := NewDecoder(strings.NewReader(input))
	var s simple
	if err := dec.Decode(&s); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if s.Name != "Eve" || s.Age != 28 {
		t.Fatalf("unexpected: %+v", s)
	}
}

func TestSkipTagField(t *testing.T) {
	input := join(
		`# INGR.io | people: $ID, name`,
		`"1"`,
		`"Alice"`,
		`# 1 record`,
	)
	dec := NewDecoder(strings.NewReader(input))
	var p personWithSkip
	if err := dec.Decode(&p); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if p.Ignored != "" {
		t.Fatalf("Ignored should be empty, got %q", p.Ignored)
	}
}

func TestNullValue(t *testing.T) {
	input := join(
		`# INGR.io | data: $ID, val`,
		`"1"`,
		`null`,
		`# 1 record`,
	)
	dec := NewDecoder(strings.NewReader(input))
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if m["val"] != nil {
		t.Fatalf("val = %v, want nil", m["val"])
	}
}

func TestCommentedOutNull(t *testing.T) {
	// '#' alone on a line = commented-out null.
	input := join(
		`# INGR.io | data: $ID`,
		`#`,
		`# 1 record`,
	)
	dec := NewDecoder(strings.NewReader(input))
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if m["$ID"] != nil {
		t.Fatalf("$ID = %v, want nil", m["$ID"])
	}
}

func TestDelimiterMinimal(t *testing.T) {
	// '#-' is the shortest valid delimiter.
	input := join(
		`# INGR.io | x: $ID`,
		`"a"`,
		`#-`,
		`"b"`,
		`#-`,
		`# 2 records`,
	)
	var recs []map[string]any
	if err := Unmarshal([]byte(input), &recs); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("got %d, want 2", len(recs))
	}
}

func TestInlineCommentStripping(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no_comment", `"hello"`, `"hello"`},
		{"simple", `true # note`, `true`},
		{"inside_string", `"a # b"`, `"a # b"`},
		{"after_string", `"val" # note`, `"val"`},
		{"hash_in_escaped_quote", `"a\"b" # c`, `"a\"b"`},
		{"no_value_just_hash", `# comment only`, ``},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripInlineComment(tt.in)
			if got != tt.want {
				t.Errorf("stripInlineComment(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsDelimiter(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"#-", true},
		{"#--", true},
		{"#-----", true},
		{"#", false},
		{"# -", false},
		{"#-a", false},
		{strings.Repeat("#", 1) + strings.Repeat("-", 79), false}, // len=80, too long
		{"#-" + strings.Repeat("-", 10), true},
	}
	for _, tt := range tests {
		if got := isDelimiter(tt.in); got != tt.want {
			t.Errorf("isDelimiter(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestIsCommentedOutLine(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{`#"alice"`, true},
		{`#123`, true},
		{`#true`, true},
		{`#`, true},       // commented-out null
		{`# text`, false}, // comment line (space after #)
		{`#-`, false},     // delimiter prefix
		{`#--`, false},    // delimiter prefix
		{``, false},
		{`"hello"`, false},
	}
	for _, tt := range tests {
		if got := isCommentedOutLine(tt.in); got != tt.want {
			t.Errorf("isCommentedOutLine(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestFooterCountRegex(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"# 1 record", true},
		{"# 2 records", true},
		{"# 100 records", true},
		{"# 0 records", true},
		{"#1 record", true},   // \s* allows zero spaces
		{"#  5  records", true},
		{"# records", false},  // no number
		{"# 1record", false},  // no space between number and "record"
		{"#-5", false},
		{`"# 1 record"`, false},
	}
	for _, tt := range tests {
		if got := isFooterCountLine(tt.in); got != tt.want {
			t.Errorf("isFooterCountLine(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestReadHeaderExplicitThenDecode(t *testing.T) {
	input := join(
		`# INGR.io | stuff: $ID, val`,
		`"a"`,
		`true`,
		`# 1 record`,
	)
	dec := NewDecoder(strings.NewReader(input))
	h, err := dec.ReadHeader()
	if err != nil {
		t.Fatalf("ReadHeader: %v", err)
	}
	if h.Recordset != "stuff" {
		t.Fatalf("recordset = %q", h.Recordset)
	}

	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if m["val"] != true {
		t.Fatalf("val = %v", m["val"])
	}
}

func TestSyntaxErrorType(t *testing.T) {
	se := &SyntaxError{Line: 5, Offset: 42, msg: "bad value"}
	want := "ingr: bad value (line 5, offset 42)"
	if se.Error() != want {
		t.Fatalf("Error() = %q, want %q", se.Error(), want)
	}
}

func TestUnmarshalTypeErrorType(t *testing.T) {
	ute := &UnmarshalTypeError{
		Value: "string", Type: reflect.TypeOf(0),
		Line: 3, Field: "age",
	}
	got := ute.Error()
	if !strings.Contains(got, "age") || !strings.Contains(got, "int") {
		t.Fatalf("Error() = %q", got)
	}
}

func TestFooterWithExtraComments(t *testing.T) {
	input := join(
		`# INGR.io | x: $ID`,
		`"1"`,
		`# 1 record`,
		`# sha256:abcdef`,
		`# generated by test`,
	)
	dec := NewDecoder(strings.NewReader(input))
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if m["$ID"] != "1" {
		t.Fatalf("$ID = %v", m["$ID"])
	}
	// Ensure More() returns false after consuming footer+comments.
	if dec.More() {
		t.Fatal("More() should be false after footer")
	}
}

func TestZeroRecords(t *testing.T) {
	input := join(
		`# INGR.io | empty: $ID, name`,
		`# 0 records`,
	)
	var recs []map[string]any
	if err := Unmarshal([]byte(input), &recs); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("got %d records, want 0", len(recs))
	}
}

func TestSingularRecord(t *testing.T) {
	input := join(
		`# INGR.io | x: $ID`,
		`"only"`,
		`# 1 record`,
	)
	var recs []map[string]any
	if err := Unmarshal([]byte(input), &recs); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("got %d records, want 1", len(recs))
	}
}

func TestCommentedOutWithInlineComment(t *testing.T) {
	// Commented-out line with an inline comment after the value.
	input := join(
		`# INGR.io | x: $ID, val`,
		`#"1"`,
		`#true # was enabled`,
		`# 1 record`,
	)
	dec := NewDecoder(strings.NewReader(input))
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if m["$ID"] != "1" {
		t.Fatalf("$ID = %v", m["$ID"])
	}
	if m["val"] != true {
		t.Fatalf("val = %v, want true", m["val"])
	}
}

func TestUseNumberInStruct(t *testing.T) {
	type rec struct {
		ID  string `ingr:"$ID"`
		Num Number `ingr:"num"`
	}
	input := join(
		`# INGR.io | data: $ID, num`,
		`"1"`,
		`999999999999999999`,
		`# 1 record`,
	)
	dec := NewDecoder(strings.NewReader(input))
	var r rec
	if err := dec.Decode(&r); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	i, err := r.Num.Int64()
	if err != nil {
		t.Fatalf("Int64: %v", err)
	}
	if i != 999999999999999999 {
		t.Fatalf("got %d", i)
	}
}

func TestComplexTypes(t *testing.T) {
	// Array and object values.
	input := join(
		`# INGR.io | data: $ID, tags, meta`,
		`"1"`,
		`["a","b","c"]`,
		`{"key":"val"}`,
		`# 1 record`,
	)
	dec := NewDecoder(strings.NewReader(input))
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	tags, ok := m["tags"].([]any)
	if !ok || len(tags) != 3 {
		t.Fatalf("tags = %v", m["tags"])
	}
	meta, ok := m["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta = %v", m["meta"])
	}
	if meta["key"] != "val" {
		t.Fatalf("meta[key] = %v", meta["key"])
	}
}
