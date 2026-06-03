package levelcfg

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sync"
)

const defaultLevelsPath = "data/levels.json"

//go:embed data/*.json
var embeddedData embed.FS

type Entry struct {
	Level       uint32 `json:"level"`
	RequiredExp int32  `json:"required_exp"`
}

type Table struct {
	entries  []Entry
	byLevel  map[uint32]Entry
	base     Entry
	maxLevel uint32
}

func LoadTableFromFS(fsys fs.FS) (*Table, error) {
	return LoadTableFromPath(fsys, defaultLevelsPath)
}

func LoadTableFromPath(fsys fs.FS, path string) (*Table, error) {
	raw, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, fmt.Errorf("read levels config: %w", err)
	}

	var entries []Entry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("decode levels config: %w", err)
	}

	table := &Table{
		entries: make([]Entry, len(entries)),
		byLevel: make(map[uint32]Entry, len(entries)),
	}
	copy(table.entries, entries)
	if err := table.validateAndBuild(); err != nil {
		return nil, err
	}
	return table, nil
}

var (
	embeddedTableOnce sync.Once
	embeddedTable     *Table
	embeddedTableErr  error
)

func LoadEmbeddedTable() (*Table, error) {
	embeddedTableOnce.Do(func() {
		embeddedTable, embeddedTableErr = LoadTableFromFS(embeddedData)
	})
	return embeddedTable, embeddedTableErr
}

func MustLoadEmbeddedTable() *Table {
	table, err := LoadEmbeddedTable()
	if err != nil {
		panic(err)
	}
	return table
}

func (t *Table) BaseLevel() uint32 {
	return t.base.Level
}

func (t *Table) Entry(level uint32) (Entry, bool) {
	entry, ok := t.byLevel[level]
	return entry, ok
}

func (t *Table) MaxLevel() uint32 {
	return t.maxLevel
}

func (t *Table) LevelForExp(exp int32) Entry {
	if len(t.entries) == 0 {
		return Entry{}
	}
	best := t.base
	for _, entry := range t.entries {
		if exp < entry.RequiredExp {
			break
		}
		best = entry
	}
	return best
}

func (t *Table) EntriesBetweenLevels(fromLevel, toLevel uint32) []Entry {
	if toLevel <= fromLevel {
		return nil
	}

	result := make([]Entry, 0, toLevel-fromLevel)
	for _, entry := range t.entries {
		if entry.Level <= fromLevel {
			continue
		}
		if entry.Level > toLevel {
			break
		}
		result = append(result, entry)
	}
	return result
}

func (t *Table) validateAndBuild() error {
	if len(t.entries) == 0 {
		return fmt.Errorf("levels config must not be empty")
	}

	var prevExp int32
	for i, entry := range t.entries {
		if entry.Level == 0 {
			return fmt.Errorf("entry %d: level must be > 0", i)
		}
		if i == 0 {
			if entry.Level != 1 {
				return fmt.Errorf("first entry must start at level 1")
			}
			if entry.RequiredExp != 0 {
				return fmt.Errorf("level 1 required_exp must be 0")
			}
			t.base = entry
		} else {
			if entry.Level != t.entries[i-1].Level+1 {
				return fmt.Errorf("entry %d: levels must be contiguous", i)
			}
			if entry.RequiredExp <= prevExp {
				return fmt.Errorf("entry %d: required_exp must be strictly increasing", i)
			}
		}

		if _, exists := t.byLevel[entry.Level]; exists {
			return fmt.Errorf("duplicate level %d", entry.Level)
		}
		t.byLevel[entry.Level] = entry
		t.maxLevel = entry.Level
		prevExp = entry.RequiredExp
	}

	return nil
}
