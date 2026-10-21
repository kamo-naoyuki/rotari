package tableview

import (
	"encoding/json"
	"hash/maphash"
	"math"
	"strings"
)

// uniqueLimit bounds the distinct values counted per column.
const uniqueLimit = 100_000

// ColumnStats summarizes one column's non-empty cells, like pandas'
// describe: how many, how many distinct, and for those that read as
// numbers, their mean, sample standard deviation, extremes, and sum.
type ColumnStats struct {
	Count  int `json:"count"`
	Unique int `json:"unique"`
	// UniqueOver reports more than uniqueLimit distinct values; Unique then
	// stops at the limit.
	UniqueOver bool     `json:"unique_over,omitempty"`
	Numeric    int      `json:"numeric"`
	Mean       *float64 `json:"mean,omitempty"`
	Std        *float64 `json:"std,omitempty"`
	Min        *float64 `json:"min,omitempty"`
	Max        *float64 `json:"max,omitempty"`
	Sum        *float64 `json:"sum,omitempty"`
}

// Description is Describe's result: one entry per column, over the rows
// the filters select.
type Description struct {
	Columns []ColumnStats `json:"columns"`
	Matched int           `json:"matched"`
	Total   int           `json:"total"`
}

// columnTally accumulates one column's statistics in one pass.
type columnTally struct {
	count, numeric int
	seen           map[uint64]struct{}
	over           bool
	// Welford's running mean and sum of squared differences.
	mean, squares, sum, min, max float64
}

// Describe summarizes every column over the rows the filters select. It
// reads the whole source once, and is cached per source version and
// filters.
func (table *Table) Describe(filters []Condition) (Description, error) {
	columns := table.source.columns()
	conditions, err := compileConditions(filters, columns)
	if err != nil {
		return Description{}, err
	}
	key, _ := json.Marshal(struct {
		Source  string
		Filters []condition
	}{table.source.key(), conditions})
	value, err := results.get("describe\x00"+string(key), func() (any, error) {
		return table.describe(columns, conditions)
	})
	if err != nil {
		return Description{}, err
	}
	return value.(Description), nil
}

func (table *Table) describe(columns []Column, conditions []condition) (Description, error) {
	seed := maphash.MakeSeed()
	tallies := make([]columnTally, len(columns))
	for index := range tallies {
		tallies[index].seen = map[uint64]struct{}{}
	}
	matched := 0
	err := table.source.scan(func(number int, cells []string) bool {
		if !matchesAll(conditions, cells) {
			return true
		}
		matched++
		for index := range min(len(cells), len(tallies)) {
			tallies[index].add(seed, cells[index])
		}
		return true
	})
	if err != nil {
		return Description{}, err
	}
	description := Description{Columns: make([]ColumnStats, len(tallies)), Matched: matched, Total: table.source.len()}
	for index, tally := range tallies {
		description.Columns[index] = tally.stats()
	}
	return description, nil
}

func (tally *columnTally) add(seed maphash.Seed, cell string) {
	if strings.TrimSpace(cell) == "" {
		return
	}
	tally.count++
	if !tally.over {
		tally.seen[maphash.String(seed, cell)] = struct{}{}
		if len(tally.seen) > uniqueLimit {
			tally.over = true
		}
	}
	value, ok := parseNumber(cell)
	if !ok {
		return
	}
	tally.numeric++
	if tally.numeric == 1 {
		tally.min, tally.max = value, value
	}
	tally.min, tally.max = math.Min(tally.min, value), math.Max(tally.max, value)
	tally.sum += value
	delta := value - tally.mean
	tally.mean += delta / float64(tally.numeric)
	tally.squares += delta * (value - tally.mean)
}

func (tally columnTally) stats() ColumnStats {
	stats := ColumnStats{Count: tally.count, Unique: min(len(tally.seen), uniqueLimit), UniqueOver: tally.over, Numeric: tally.numeric}
	if tally.numeric == 0 {
		return stats
	}
	// A sum or spread past float64's range is left out rather than shown
	// as infinite.
	number := func(value float64) *float64 {
		if math.IsInf(value, 0) || math.IsNaN(value) {
			return nil
		}
		return &value
	}
	stats.Mean, stats.Min, stats.Max, stats.Sum = number(tally.mean), number(tally.min), number(tally.max), number(tally.sum)
	if tally.numeric > 1 {
		stats.Std = number(math.Sqrt(tally.squares / float64(tally.numeric-1)))
	}
	return stats
}
