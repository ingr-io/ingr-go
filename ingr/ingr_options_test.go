package ingr

import "testing"

func TestApplyOptions(t *testing.T) {
	t.Run("no_options", func(t *testing.T) {
		cfg := &ExportOptions{}
		ApplyOptions(cfg)
		if cfg.IncludeHash {
			t.Error("ApplyOptions() with no opts should not set IncludeHash")
		}
		if cfg.RecordsDelimiter {
			t.Error("ApplyOptions() with no opts should not set RecordsDelimiter")
		}
	})

	t.Run("with_hash", func(t *testing.T) {
		cfg := &ExportOptions{}
		ApplyOptions(cfg, WithHash())
		if !cfg.IncludeHash {
			t.Error("WithHash() should set IncludeHash = true")
		}
		if cfg.RecordsDelimiter {
			t.Error("WithHash() should not affect RecordsDelimiter")
		}
	})

	t.Run("with_records_delimiter", func(t *testing.T) {
		cfg := &ExportOptions{}
		ApplyOptions(cfg, WithRecordsDelimiter())
		if cfg.IncludeHash {
			t.Error("WithRecordsDelimiter() should not affect IncludeHash")
		}
		if !cfg.RecordsDelimiter {
			t.Error("WithRecordsDelimiter() should set RecordsDelimiter = true")
		}
	})

	t.Run("multiple_options", func(t *testing.T) {
		cfg := &ExportOptions{}
		ApplyOptions(cfg, WithHash(), WithRecordsDelimiter())
		if !cfg.IncludeHash {
			t.Error("ApplyOptions() should set IncludeHash")
		}
		if !cfg.RecordsDelimiter {
			t.Error("ApplyOptions() should set RecordsDelimiter")
		}
	})
}
