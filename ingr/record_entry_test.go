package ingr

import "testing"

func TestNewMapRecordEntry(t *testing.T) {
	data := map[string]any{"name": "Alice", "age": 30}
	r := NewMapRecordEntry("id-1", data)
	if r == nil {
		t.Fatal("NewMapRecordEntry() returned nil")
	}
}

func TestMapRecordEntry_GetID(t *testing.T) {
	t.Run("string_id", func(t *testing.T) {
		r := NewMapRecordEntry("abc", nil)
		if got := r.GetID(); got != "abc" {
			t.Errorf("GetID() = %q, want %q", got, "abc")
		}
	})

	t.Run("int_id", func(t *testing.T) {
		r := NewMapRecordEntry(42, nil)
		if got := r.GetID(); got != "42" {
			t.Errorf("GetID() = %q, want %q", got, "42")
		}
	})
}

func TestMapRecordEntry_GetData(t *testing.T) {
	t.Run("with_data", func(t *testing.T) {
		data := map[string]any{"foo": "bar"}
		r := NewMapRecordEntry("x", data)
		me := r.(mapRecordEntry[string])
		got := me.GetData()
		if got == nil {
			t.Fatal("GetData() returned nil")
		}
		if got["foo"] != "bar" {
			t.Errorf("GetData()[\"foo\"] = %v, want %q", got["foo"], "bar")
		}
	})

	t.Run("nil_data", func(t *testing.T) {
		r := NewMapRecordEntry("x", nil)
		me := r.(mapRecordEntry[string])
		got := me.GetData()
		if got != nil {
			t.Errorf("GetData() = %v, want nil", got)
		}
	})
}

func TestMapRecordEntry_GetValue_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("GetValue() should have panicked")
		}
	}()
	r := NewMapRecordEntry("x", nil)
	r.GetValue("foo")
}

func TestMapRecordEntry_GetIntValue_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("GetIntValue() should have panicked")
		}
	}()
	r := NewMapRecordEntry("x", nil)
	r.GetIntValue("foo")
}

func TestMapRecordEntry_GetStrValue_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("GetStrValue() should have panicked")
		}
	}()
	r := NewMapRecordEntry("x", nil)
	r.GetStrValue("foo")
}

func TestMapRecordEntry_GetBoolValue_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("GetBoolValue() should have panicked")
		}
	}()
	r := NewMapRecordEntry("x", nil)
	r.GetBoolValue("foo")
}

func TestMapRecordEntry_IsCommented_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("IsCommented() should have panicked")
		}
	}()
	r := NewMapRecordEntry("x", nil)
	r.IsCommented()
}
