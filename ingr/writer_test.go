package ingr

import (
	"bytes"
	"strings"
	"testing"
)

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
}

func TestWriteHeader(t *testing.T) {
	cols := []ColDef{
		{Name: "$ID", Type: "string"},
		{Name: "name", Type: "string"},
		{Name: "age", Type: "int"},
	}
	w := &bytes.Buffer{}
	rw := NewRecordsWriter(w)
	_, err := rw.WriteHeader("people", cols)
	if err != nil {
		t.Fatalf("WriteHeader: %v", err)
	}
	want := "# INGR.io | people: $ID:string, name:string, age:int\n"
	if got := w.String(); got != want {
		t.Errorf("header =\n%q\nwant\n%q", got, want)
	}
}

func TestWriteHeaderTwiceFails(t *testing.T) {
	w := &bytes.Buffer{}
	rw := NewRecordsWriter(w)
	cols := []ColDef{{Name: "$ID"}}
	if _, err := rw.WriteHeader("x", cols); err != nil {
		t.Fatalf("first WriteHeader: %v", err)
	}
	if _, err := rw.WriteHeader("x", cols); err == nil {
		t.Fatal("expected error on second WriteHeader, got nil")
	}
}

func TestWriteRecordsWithoutHeaderFails(t *testing.T) {
	w := &bytes.Buffer{}
	rw := NewRecordsWriter(w)
	r := NewMapRecordEntry("1", map[string]any{})
	_, err := rw.WriteRecords(0, r)
	if err == nil {
		t.Fatal("expected error when header not written, got nil")
	}
}

func TestWriteRecordsFullOutput(t *testing.T) {
	w := &bytes.Buffer{}
	rw := NewRecordsWriter(w)
	cols := []ColDef{{Name: "$ID"}, {Name: "name"}, {Name: "active"}}
	rw.WriteHeader("people", cols)

	r1 := NewMapRecordEntry("alice", map[string]any{"name": "Alice Smith", "active": true})
	r2 := NewMapRecordEntry("bob", map[string]any{"name": "Bob Jones", "active": false})
	if _, err := rw.WriteRecords(0, r1, r2); err != nil {
		t.Fatalf("WriteRecords: %v", err)
	}
	if err := rw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	want := strings.Join([]string{
		`# INGR.io | people: $ID, name, active`,
		`"alice"`,
		`"Alice Smith"`,
		`true`,
		`"bob"`,
		`"Bob Jones"`,
		`false`,
		`# 2 records`,
	}, "\n")
	if got := w.String(); got != want {
		t.Errorf("output =\n%q\nwant\n%q", got, want)
	}
}

func TestWriteRecordsWithDelimiter(t *testing.T) {
	w := &bytes.Buffer{}
	rw := NewRecordsWriter(w)
	cols := []ColDef{{Name: "$ID"}, {Name: "v"}}
	rw.WriteHeader("x", cols)

	r := NewMapRecordEntry("1", map[string]any{"v": 42})
	rw.WriteRecords(1, r)
	rw.Close()

	out := w.String()
	if !strings.Contains(out, "#-\n") {
		t.Errorf("expected delimiter line '#-', got:\n%s", out)
	}
}

func TestFooterSingularPlural(t *testing.T) {
	cols := []ColDef{{Name: "$ID"}}
	for _, tc := range []struct {
		records int
		want    string
	}{
		{0, "# 0 records"},
		{1, "# 1 record"},
		{2, "# 2 records"},
	} {
		w := &bytes.Buffer{}
		rw := NewRecordsWriter(w)
		rw.WriteHeader("x", cols)
		for i := 0; i < tc.records; i++ {
			rw.WriteRecords(0, NewMapRecordEntry(i, map[string]any{}))
		}
		rw.Close()
		out := w.String()
		if !strings.Contains(out, tc.want) {
			t.Errorf("records=%d: want footer %q in:\n%s", tc.records, tc.want, out)
		}
	}
}

func TestRecordEntrySetComment(t *testing.T) {
	r := NewMapRecordEntry("id1", map[string]any{"name": "Alice"})
	if err := r.SetComment("name", "primary user"); err != nil {
		t.Fatalf("SetComment: %v", err)
	}
	if got := r.GetComment("name"); got != "primary user" {
		t.Errorf("GetComment = %q, want %q", got, "primary user")
	}
}

func TestRecordEntrySetValue(t *testing.T) {
	r := NewMapRecordEntry("id1", map[string]any{"age": 30})
	if err := r.SetValue("age", 31); err != nil {
		t.Fatalf("SetValue: %v", err)
	}
	if got := r.GetIntValue("age"); got != 31 {
		t.Errorf("GetIntValue = %d, want 31", got)
	}
}

func TestRecordEntrySetIsCommentedOut(t *testing.T) {
	r := NewMapRecordEntry("id1", map[string]any{})
	if r.GetIsCommentedOut() {
		t.Fatal("expected false before SetIsCommentedOut")
	}
	r.SetIsCommentedOut()
	if !r.GetIsCommentedOut() {
		t.Fatal("expected true after SetIsCommentedOut")
	}
}
