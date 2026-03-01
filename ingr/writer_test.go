package ingr

import (
	"bytes"
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
