package bankimport

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// QIF reads Quicken Interchange Format bank/cash statements. Records are
// newline-delimited single-letter fields terminated by a "^" line; a leading
// "!Type:..." header is ignored. QIF carries no stable transaction id, so
// duplicate detection falls back to a content hash (see app.importRef).
type QIF struct{}

// Read parses QIF from r. It returns an error if no transactions are found
// (most often a wrong-format upload).
func (QIF) Read(r io.Reader) ([]app.StatementTxn, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var out []app.StatementTxn
	var cur app.StatementTxn
	var payee, memo string
	var started, hasAmount, hasDate bool

	flush := func() error {
		if !started {
			return nil
		}
		if !hasDate || !hasAmount {
			return fmt.Errorf("QIF record missing date (D) or amount (T)")
		}
		cur.Memo = joinNonEmpty(payee, memo)
		out = append(out, cur)
		cur, payee, memo = app.StatementTxn{}, "", ""
		started, hasAmount, hasDate = false, false, false
		return nil
	}

	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "" || strings.HasPrefix(line, "!") {
			continue
		}
		if line == "^" {
			if err := flush(); err != nil {
				return nil, err
			}
			continue
		}
		started = true
		code, val := line[0], strings.TrimSpace(line[1:])
		switch code {
		case 'D':
			d, err := parseQIFDate(val)
			if err != nil {
				return nil, err
			}
			cur.Date, hasDate = d, true
		case 'T', 'U':
			amt, err := domain.FromDecimalString(strings.ReplaceAll(val, ",", ""))
			if err != nil {
				return nil, fmt.Errorf("invalid QIF amount %q: %w", val, err)
			}
			cur.Amount, hasAmount = amt, true
		case 'P':
			payee = val
		case 'M':
			memo = val
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	// A final record may not be followed by a "^".
	if err := flush(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no transactions found (is this a QIF file?)")
	}
	return out, nil
}

// qifDateLayouts covers the common Quicken date spellings. The apostrophe form
// ("6/19'24") is normalised to a slash before matching.
var qifDateLayouts = []string{
	"1/2/2006", "01/02/2006", "1/2/06", "01/02/06",
	"2006-1-2", "2006-01-02", "1-2-2006",
}

func parseQIFDate(s string) (time.Time, error) {
	norm := strings.ReplaceAll(strings.ReplaceAll(s, "'", "/"), " ", "")
	for _, layout := range qifDateLayouts {
		if t, err := time.Parse(layout, norm); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised QIF date %q", s)
}
