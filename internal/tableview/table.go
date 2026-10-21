package tableview

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// Column is one column of a table.
type Column struct {
	Name string `json:"name"`
}

// source is where a table's rows come from: a delimited file read from
// disk, or a directory listing held in memory.
type source interface {
	// key identifies the source's contents, for caching what is computed
	// from them.
	key() string
	columns() []Column
	len() int
	// scan visits every row in order until visit returns false.
	scan(visit func(number int, cells []string) bool) error
	// rows returns the rows numbered, which ascend.
	rows(numbers []int) ([][]string, error)
}

// Table is a source of rows the viewer pages through.
type Table struct {
	source    source
	target    Target
	directory bool
}

// OpenFile reads target as a delimited file.
func OpenFile(target Target, format Format) (*Table, error) {
	source, err := openDelimited(target, format)
	if err != nil {
		return nil, err
	}
	return &Table{source: source, target: target}, nil
}

// Columns names the table's columns.
func (table *Table) Columns() []Column { return table.source.columns() }

// Limits on one page.
const (
	DefaultLimit = 100
	MaxLimit     = 1000
	// cellLimit is the most bytes of one cell a page carries.
	cellLimit = 4096
)

// Query selects a page of rows: those every filter matches, in the sort's
// order or the source's.
type Query struct {
	Filters []Condition
	Sort    *Sort
	Offset  int
	Limit   int
}

// Page is one page of a table.
type Page struct {
	Columns []Column `json:"columns"`
	Rows    []Row    `json:"rows"`
	Offset  int      `json:"offset"`
	// Matched counts the rows the query selects; Total counts them all.
	Matched int `json:"matched"`
	Total   int `json:"total"`
	// Cut reports cells shortened to cellLimit bytes.
	Cut bool `json:"cut,omitempty"`
}

// Row is a row of a page; Number counts data rows from 1 in the source.
type Row struct {
	Number int      `json:"number"`
	Cells  []string `json:"cells"`
}

// Page reads the rows a query selects.
func (table *Table) Page(query Query) (Page, error) {
	if query.Offset < 0 {
		return Page{}, fmt.Errorf("offset must not be negative")
	}
	if query.Limit == 0 {
		query.Limit = DefaultLimit
	}
	if query.Limit < 1 || query.Limit > MaxLimit {
		return Page{}, fmt.Errorf("limit must be between 1 and %d", MaxLimit)
	}
	columns := table.source.columns()
	conditions, err := compileConditions(query.Filters, columns)
	if err != nil {
		return Page{}, err
	}
	selected, err := table.selection(conditions, query.Sort)
	if err != nil {
		return Page{}, err
	}
	total := table.source.len()
	page := Page{Columns: columns, Rows: []Row{}, Offset: query.Offset, Matched: total, Total: total}
	var numbers []int
	if selected == nil {
		for number := query.Offset; number < min(total, query.Offset+query.Limit); number++ {
			numbers = append(numbers, number)
		}
	} else {
		page.Matched = len(selected)
		numbers = selected[min(query.Offset, len(selected)):min(query.Offset+query.Limit, len(selected))]
	}
	// The source reads ascending numbers; a sorted page is put back in
	// order after.
	ascending := slices.Sorted(slices.Values(numbers))
	cells, err := table.source.rows(ascending)
	if err != nil {
		return Page{}, err
	}
	byNumber := make(map[int][]string, len(cells))
	for index, row := range cells {
		byNumber[ascending[index]] = row
	}
	for _, number := range numbers {
		row := byNumber[number]
		for cell, value := range row {
			if len(value) > cellLimit {
				row[cell], page.Cut = cutUTF8(value, cellLimit), true
			}
		}
		page.Rows = append(page.Rows, Row{Number: number + 1, Cells: row})
	}
	return page, nil
}

// cutUTF8 shortens text to at most limit bytes without splitting a rune.
func cutUTF8(text string, limit int) string {
	for limit > 0 && !utf8.RuneStart(text[limit]) {
		limit--
	}
	return strings.Clone(text[:limit]) + "…"
}
