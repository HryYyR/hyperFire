package levelcfg

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadEmbeddedTable(t *testing.T) {
	table, err := LoadEmbeddedTable()
	if err != nil {
		t.Fatalf("expected embedded level table to load, got error: %v", err)
	}

	if got := table.BaseLevel(); got != 1 {
		t.Fatalf("expected base level 1, got %d", got)
	}

	level := table.LevelForExp(12)
	if level.Level != 2 {
		t.Fatalf("expected exp 12 to resolve level 2, got %d", level.Level)
	}
	if got := level.RequiredExp; got != 10 {
		t.Fatalf("expected level 2 required exp 10, got %d", got)
	}
}

func TestLoadTableRejectsNonContiguousLevels(t *testing.T) {
	fsys := fstest.MapFS{
		defaultLevelsPath: &fstest.MapFile{Data: []byte(`[
			{"level":1,"required_exp":0},
			{"level":3,"required_exp":5}
		]`)},
	}

	_, err := LoadTableFromFS(fsys)
	if err == nil {
		t.Fatal("expected level loading to fail")
	}
	if !strings.Contains(err.Error(), "contiguous") {
		t.Fatalf("expected contiguous validation error, got: %v", err)
	}
}
