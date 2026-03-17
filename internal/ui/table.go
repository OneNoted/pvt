package ui

import (
	"fmt"
	"io"
	"strings"
)

// Table renders aligned columnar output.
type Table struct {
	headers []string
	rows    [][]string
}

// NewTable creates a table with the given column headers.
func NewTable(headers ...string) *Table {
	return &Table{headers: headers}
}

// AddRow adds a row to the table.
func AddRow(t *Table, values ...string) {
	t.rows = append(t.rows, values)
}

// Render writes the table to the given writer.
func (t *Table) Render(w io.Writer) {
	widths := make([]int, len(t.headers))
	for i, h := range t.headers {
		widths[i] = len(h)
	}
	for _, row := range t.rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	// Header
	for i, h := range t.headers {
		if i > 0 {
			fmt.Fprint(w, "  ")
		}
		fmt.Fprintf(w, "%-*s", widths[i], strings.ToUpper(h))
	}
	fmt.Fprintln(w)

	// Rows
	for _, row := range t.rows {
		for i, cell := range row {
			if i > 0 {
				fmt.Fprint(w, "  ")
			}
			if i < len(widths) {
				fmt.Fprintf(w, "%-*s", widths[i], cell)
			} else {
				fmt.Fprint(w, cell)
			}
		}
		fmt.Fprintln(w)
	}
}
