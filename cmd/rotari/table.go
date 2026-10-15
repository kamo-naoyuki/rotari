package main

import (
	"fmt"
	"io"
	"strings"
)

// textTable is a table of the CLI's list views. Each column is as wide as its
// longest value, so long values keep the later columns aligned.
type textTable struct {
	columns []tableColumn
	rows    [][]string
	// wide prints every column in full, as --wide asks. Otherwise wideOnly
	// columns are left out, and a foldable column with one value in every row
	// is printed once above the table, followed by "(all <rowNoun>)".
	wide    bool
	rowNoun string
}

type tableColumn struct {
	header     string
	alignRight bool
	wideOnly   bool
	// fold labels the column when it is folded; a folded "-" is not printed.
	fold string
	// color, when set, colors a cell; widths count the plain value.
	color func(row int, value string) string
}

func (table textTable) write(writer io.Writer) {
	shown := make([]int, 0, len(table.columns))
	widths := make([]int, len(table.columns))
	header := make([]string, len(table.columns))
	for index, column := range table.columns {
		if !table.wide && column.wideOnly {
			continue
		}
		if value, folded := table.folded(index); folded {
			if value != "-" {
				fmt.Fprintf(writer, "%s %s (all %s)\n", cyan(column.fold+":"), value, table.rowNoun)
			}
			continue
		}
		shown = append(shown, index)
		header[index] = column.header
		widths[index] = len(column.header)
		for _, row := range table.rows {
			widths[index] = max(widths[index], len(row[index]))
		}
	}
	fmt.Fprintln(writer, cyan(table.line(shown, widths, header, -1)))
	for rowIndex, row := range table.rows {
		fmt.Fprintln(writer, table.line(shown, widths, row, rowIndex))
	}
}

// folded reports the one value of a foldable column. A single row is not
// folded, so a one-job table keeps its columns.
func (table textTable) folded(index int) (string, bool) {
	if table.wide || table.columns[index].fold == "" || len(table.rows) < 2 {
		return "", false
	}
	value := table.rows[0][index]
	for _, row := range table.rows[1:] {
		if row[index] != value {
			return "", false
		}
	}
	return value, true
}

// line lays out one row; rowIndex is -1 for the header, which is not colored
// per cell.
func (table textTable) line(shown, widths []int, values []string, rowIndex int) string {
	var builder strings.Builder
	for position, index := range shown {
		column, value := table.columns[index], values[index]
		padding := strings.Repeat(" ", widths[index]-len(value))
		cell := value
		if rowIndex >= 0 && column.color != nil {
			cell = column.color(rowIndex, value)
		}
		if position > 0 {
			builder.WriteString("  ")
		}
		switch {
		case column.alignRight:
			builder.WriteString(padding + cell)
		case position < len(shown)-1:
			builder.WriteString(cell + padding)
		default:
			builder.WriteString(cell)
		}
	}
	return strings.TrimRight(builder.String(), " ")
}
