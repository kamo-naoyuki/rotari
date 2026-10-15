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
}

type tableColumn struct {
	header     string
	alignRight bool
	// color, when set, colors a cell; widths count the plain value.
	color func(row int, value string) string
}

func (table textTable) write(writer io.Writer) {
	shown := make([]int, len(table.columns))
	widths := make([]int, len(table.columns))
	header := make([]string, len(table.columns))
	for index, column := range table.columns {
		shown[index] = index
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
