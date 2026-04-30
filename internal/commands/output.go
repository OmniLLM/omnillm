package commands

import (
	"fmt"
	"io"
	"strings"
)

type Table struct {
	headers []string
	rows    [][]string
	widths  []int
}

func NewTable(headers ...string) *Table {
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = len(header)
	}

	return &Table{
		headers: headers,
		widths:  widths,
	}
}

func (t *Table) AddRow(columns ...string) {
	row := make([]string, len(t.headers))
	copy(row, columns)

	for i, value := range row {
		if len(value) > t.widths[i] {
			t.widths[i] = len(value)
		}
	}

	t.rows = append(t.rows, row)
}

func (t *Table) Render(w io.Writer) error {
	if len(t.headers) == 0 {
		return nil
	}

	headerLine := t.formatRow(t.headers)
	if _, err := fmt.Fprintln(w, headerLine); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, strings.Repeat("─", len(headerLine))); err != nil {
		return err
	}

	for _, row := range t.rows {
		if _, err := fmt.Fprintln(w, t.formatRow(row)); err != nil {
			return err
		}
	}

	return nil
}

func (t *Table) formatRow(columns []string) string {
	formatted := make([]string, len(t.headers))
	for i := range t.headers {
		value := ""
		if i < len(columns) {
			value = columns[i]
		}
		if i == len(t.headers)-1 {
			formatted[i] = value
			continue
		}
		formatted[i] = padRight(value, t.widths[i])
	}

	return strings.Join(formatted, "  ")
}

func PrintSection(w io.Writer, title string) error {
	if _, err := fmt.Fprintln(w, title); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w, strings.Repeat("─", len(title)))
	return err
}

func PrintKeyValue(w io.Writer, key string, value any) error {
	_, err := fmt.Fprintf(w, "%-16s %v\n", key+":", value)
	return err
}

func PrintEmpty(w io.Writer, entity string) error {
	_, err := fmt.Fprintf(w, "No %s.\n", entity)
	return err
}
