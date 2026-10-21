package tableview

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

// Condition selects rows by the cell in one column, counted from 0. A row
// shorter than the column has an empty cell there.
type Condition struct {
	Column int    `json:"column"`
	Op     string `json:"op"`
	Value  string `json:"value,omitempty"`
}

// The operators a Condition takes. Equality compares numbers when the cell
// and the value both read as one, and text otherwise; the orderings match
// only cells that read as numbers. Contains compares text, case-sensitive.
const (
	OpEqual        = "="
	OpNotEqual     = "!="
	OpLess         = "<"
	OpLessEqual    = "<="
	OpGreater      = ">"
	OpGreaterEqual = ">="
	OpContains     = "contains"
	OpNotContains  = "not_contains"
	OpEmpty        = "empty"
	OpNotEmpty     = "not_empty"
)

// ParseConditions reads conditions from their JSON list.
func ParseConditions(text string) ([]Condition, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	var conditions []Condition
	if err := json.Unmarshal([]byte(text), &conditions); err != nil {
		return nil, fmt.Errorf("filters must be a JSON list of {column, op, value}: %w", err)
	}
	return conditions, nil
}

// condition is a Condition checked against the table's columns.
type condition struct {
	Condition
	number  float64
	numeric bool
	// text orders by text: a time column's RFC 3339 cells.
	text bool
}

func compileConditions(conditions []Condition, columns []Column) ([]condition, error) {
	compiled := make([]condition, 0, len(conditions))
	for _, given := range conditions {
		if given.Column < 0 || given.Column >= len(columns) {
			return nil, fmt.Errorf("a filter names column %d; columns are counted from 0 and the table has %d", given.Column, len(columns))
		}
		check := condition{Condition: given}
		check.number, check.numeric = parseValue(columns[given.Column].Type, given.Value)
		check.text = columns[given.Column].Type == TypeTime
		switch given.Op {
		case OpEqual, OpNotEqual, OpContains, OpNotContains:
		case OpLess, OpLessEqual, OpGreater, OpGreaterEqual:
			if check.text && given.Value != "" {
				break
			}
			if !check.numeric {
				return nil, fmt.Errorf("%s %s needs a number, not %q", columns[given.Column].Name, given.Op, given.Value)
			}
		case OpEmpty, OpNotEmpty:
			check.Value = ""
		default:
			return nil, fmt.Errorf("unknown filter operator %q", given.Op)
		}
		compiled = append(compiled, check)
	}
	return compiled, nil
}

// parseValue reads a filter's value as a number in a column's units: a size
// may carry a binary suffix (K, M, G, T, with an optional "iB" or "B"),
// and a duration may be h:mm:ss or m:ss as well as seconds.
func parseValue(columnType, text string) (float64, bool) {
	text = strings.TrimSpace(text)
	switch columnType {
	case TypeBytes:
		upper := strings.TrimSuffix(strings.TrimSuffix(strings.ToUpper(text), "B"), "I")
		for power, suffix := range []string{"K", "M", "G", "T"} {
			if number, ok := strings.CutSuffix(upper, suffix); ok {
				value, ok := parseNumber(number)
				return value * math.Pow(1024, float64(power+1)), ok
			}
		}
	case TypeDuration:
		if strings.Contains(text, ":") {
			seconds := 0.0
			for _, part := range strings.Split(text, ":") {
				value, ok := parseNumber(part)
				if !ok || value < 0 {
					return 0, false
				}
				seconds = seconds*60 + value
			}
			return seconds, true
		}
	}
	return parseNumber(text)
}

// parseNumber reads a finite number, ignoring surrounding spaces.
func parseNumber(text string) (float64, bool) {
	value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return value, true
}

func (check condition) matches(cells []string) bool {
	cell := ""
	if check.Column < len(cells) {
		cell = cells[check.Column]
	}
	switch check.Op {
	case OpEmpty:
		return strings.TrimSpace(cell) == ""
	case OpNotEmpty:
		return strings.TrimSpace(cell) != ""
	case OpContains:
		return strings.Contains(cell, check.Value)
	case OpNotContains:
		return !strings.Contains(cell, check.Value)
	}
	if check.text {
		order := strings.Compare(cell, check.Value)
		switch check.Op {
		case OpLess:
			return cell != "" && order < 0
		case OpLessEqual:
			return cell != "" && order <= 0
		case OpGreater:
			return order > 0
		case OpGreaterEqual:
			return order >= 0
		}
	}
	number, numeric := parseNumber(cell)
	switch check.Op {
	case OpEqual, OpNotEqual:
		equal := cell == check.Value
		if numeric && check.numeric {
			equal = number == check.number
		}
		return equal == (check.Op == OpEqual)
	}
	if !numeric {
		return false
	}
	switch check.Op {
	case OpLess:
		return number < check.number
	case OpLessEqual:
		return number <= check.number
	case OpGreater:
		return number > check.number
	default:
		return number >= check.number
	}
}

func matchesAll(conditions []condition, cells []string) bool {
	for _, check := range conditions {
		if !check.matches(cells) {
			return false
		}
	}
	return true
}

// Sort orders rows by one column. Cells that read as numbers come first,
// by value, then the rest by text; equal cells keep the file's order.
type Sort struct {
	Column     int
	Descending bool
}

// Bounds on what a query keeps in memory.
const (
	// maxMatches bounds the row numbers a filter keeps.
	maxMatches = 10_000_000
	// maxSorted bounds the rows a sort orders.
	maxSorted = 1_000_000
)

// selection returns the numbers of the rows a query selects, in order, or
// nil for every row in the source's order. It is cached per source
// version, filters, and sort.
func (table *Table) selection(conditions []condition, sort *Sort) ([]int, error) {
	if len(conditions) == 0 && sort == nil {
		return nil, nil
	}
	key, _ := json.Marshal(struct {
		Source  string
		Filters []condition
		Sort    *Sort
	}{table.source.key(), conditions, sort})
	value, err := results.get("selection\x00"+string(key), func() (any, error) {
		if sort == nil {
			return table.matches(conditions)
		}
		return table.sorted(conditions, *sort)
	})
	if err != nil {
		return nil, err
	}
	return value.([]int), nil
}

func (table *Table) matches(conditions []condition) ([]int, error) {
	numbers := []int{}
	var tooMany bool
	err := table.source.scan(func(number int, cells []string) bool {
		if matchesAll(conditions, cells) {
			if len(numbers) == maxMatches {
				tooMany = true
				return false
			}
			numbers = append(numbers, number)
		}
		return true
	})
	if tooMany {
		return nil, fmt.Errorf("more than %d rows match; add a filter", maxMatches)
	}
	return numbers, err
}

func (table *Table) sorted(conditions []condition, sort Sort) ([]int, error) {
	if sort.Column < 0 || sort.Column >= len(table.source.columns()) {
		return nil, fmt.Errorf("sort names column %d; the table has %d", sort.Column, len(table.source.columns()))
	}
	type keyed struct {
		number  int
		text    string
		value   float64
		numeric bool
	}
	var rows []keyed
	var tooMany bool
	err := table.source.scan(func(number int, cells []string) bool {
		if !matchesAll(conditions, cells) {
			return true
		}
		if len(rows) == maxSorted {
			tooMany = true
			return false
		}
		row := keyed{number: number}
		if sort.Column < len(cells) {
			row.text = cells[sort.Column]
		}
		row.value, row.numeric = parseNumber(row.text)
		rows = append(rows, row)
		return true
	})
	if err != nil {
		return nil, err
	}
	if tooMany {
		return nil, fmt.Errorf("sorting is limited to %d rows; add a filter", maxSorted)
	}
	slices.SortStableFunc(rows, func(a, b keyed) int {
		var order int
		switch {
		case a.numeric && b.numeric:
			order = cmp.Compare(a.value, b.value)
		case a.numeric != b.numeric:
			// Numbers first in either direction, so blanks stay last.
			if a.numeric {
				return -1
			}
			return 1
		default:
			order = strings.Compare(a.text, b.text)
		}
		if sort.Descending {
			order = -order
		}
		return order
	})
	numbers := make([]int, len(rows))
	for index, row := range rows {
		numbers[index] = row.number
	}
	return numbers, nil
}
