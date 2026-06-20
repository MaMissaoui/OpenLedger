package bankimport

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// CSVMapping describes how a CSV's columns map onto a statement line. Columns
// are zero-based; a negative index means "unused". Amounts come either from a
// single signed AmountCol, or from separate DebitCol (money out) and CreditCol
// (money in) columns.
type CSVMapping struct {
	HasHeader  bool
	DateCol    int
	DateFormat string // a Go time layout; empty falls back to common layouts
	DescCols   []int
	AmountCol  int
	DebitCol   int
	CreditCol  int
	Invert     bool // negate the resulting amount (banks that list outflows positive)
}

// CSV reads delimited bank exports using a caller-supplied column Mapping. It is
// built per request (the mapping comes from the import wizard), so it is not a
// static entry in the readers map.
type CSV struct {
	Mapping CSVMapping
}

// Read parses CSV from r per the mapping. Quoted fields and embedded commas are
// handled by encoding/csv. Blank rows are skipped; an error is returned if no
// data rows remain or a mapped cell cannot be parsed.
func (c CSV) Read(r io.Reader) ([]app.StatementTxn, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1 // tolerate ragged rows
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	if c.Mapping.HasHeader && len(rows) > 0 {
		rows = rows[1:]
	}

	out := make([]app.StatementTxn, 0, len(rows))
	for i, row := range rows {
		if isBlankRow(row) {
			continue
		}
		date, err := c.parseDate(cell(row, c.Mapping.DateCol))
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", i+1, err)
		}
		amount, err := c.parseAmount(row)
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", i+1, err)
		}
		desc := make([]string, 0, len(c.Mapping.DescCols))
		for _, col := range c.Mapping.DescCols {
			desc = append(desc, cell(row, col))
		}
		out = append(out, app.StatementTxn{
			Date:   date,
			Amount: amount,
			Memo:   joinNonEmpty(desc...),
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no data rows found in CSV")
	}
	return out, nil
}

func (c CSV) parseAmount(row []string) (domain.GncNumeric, error) {
	if c.Mapping.AmountCol >= 0 {
		amt, err := parseMoney(cell(row, c.Mapping.AmountCol))
		if err != nil {
			return domain.GncNumeric{}, err
		}
		return c.signed(amt), nil
	}
	// Debit/credit columns: debit is money out (negative), credit money in.
	debit, err := parseMoney(cell(row, c.Mapping.DebitCol))
	if err != nil {
		return domain.GncNumeric{}, err
	}
	credit, err := parseMoney(cell(row, c.Mapping.CreditCol))
	if err != nil {
		return domain.GncNumeric{}, err
	}
	return c.signed(credit.Sub(debit)), nil
}

func (c CSV) signed(n domain.GncNumeric) domain.GncNumeric {
	if c.Mapping.Invert {
		return n.Neg()
	}
	return n
}

// csvDateLayouts are tried when the mapping leaves DateFormat blank.
var csvDateLayouts = []string{
	"2006-01-02", "01/02/2006", "1/2/2006", "02/01/2006",
	"2006/01/02", "02-01-2006", "01-02-2006", "Jan 2, 2006", "2 Jan 2006",
}

func (c CSV) parseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty date")
	}
	if f := strings.TrimSpace(c.Mapping.DateFormat); f != "" {
		t, err := time.Parse(f, s)
		if err != nil {
			return time.Time{}, fmt.Errorf("date %q does not match format %q", s, f)
		}
		return t, nil
	}
	for _, layout := range csvDateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised date %q", s)
}

// parseMoney parses a money cell into an exact amount. It tolerates a leading
// currency symbol, thousands separators (US-style commas), surrounding spaces,
// and accounting-style parentheses for negatives. An empty cell is zero.
func parseMoney(s string) (domain.GncNumeric, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return domain.FromNumDenom(0, 1)
	}
	neg := false
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		neg, s = true, s[1:len(s)-1]
	}
	// Drop currency symbols, spaces, and thousands separators.
	s = strings.Map(func(r rune) rune {
		switch r {
		case '$', '€', '£', '¥', ',', ' ', ' ':
			return -1
		}
		return r
	}, s)
	n, err := domain.FromDecimalString(s)
	if err != nil {
		return domain.GncNumeric{}, fmt.Errorf("invalid amount %q", strings.TrimSpace(s))
	}
	if neg {
		return n.Neg(), nil
	}
	return n, nil
}

func cell(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return row[idx]
}

func isBlankRow(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

// CSVPreview is a sample of an uploaded CSV used to drive the mapping wizard:
// the first rows verbatim (the client decides which is a header), the total row
// count, and the widest row's column count.
type CSVPreview struct {
	Rows      [][]string
	TotalRows int
	Columns   int
}

// PreviewCSV returns up to maxRows rows from r plus totals, for the mapping UI.
func PreviewCSV(r io.Reader, maxRows int) (CSVPreview, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	all, err := cr.ReadAll()
	if err != nil {
		return CSVPreview{}, err
	}
	cols := 0
	for _, row := range all {
		if len(row) > cols {
			cols = len(row)
		}
	}
	rows := all
	if maxRows > 0 && len(rows) > maxRows {
		rows = rows[:maxRows]
	}
	return CSVPreview{Rows: rows, TotalRows: len(all), Columns: cols}, nil
}
