package bankimport

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// OFX reads Open Financial Exchange statements. It tolerates both OFX 1.x
// (SGML, unclosed value tags) and OFX 2.x (XML) by scanning each
// <STMTTRN>…</STMTTRN> block and reading each field's value up to the next tag
// or end of line.
type OFX struct{}

var (
	stmtTrnRe = regexp.MustCompile(`(?is)<STMTTRN>(.*?)</STMTTRN>`)
	// One capture: the value between <TAG> and the next '<' or line break. The
	// %s is the tag name; values are trimmed by the caller.
	ofxFieldRe = func(tag string) *regexp.Regexp {
		return regexp.MustCompile(`(?i)<` + tag + `>([^<\r\n]*)`)
	}
	dtPostedRe = ofxFieldRe("DTPOSTED")
	trnAmtRe   = ofxFieldRe("TRNAMT")
	fitIDRe    = ofxFieldRe("FITID")
	nameRe     = ofxFieldRe("NAME")
	memoRe     = ofxFieldRe("MEMO")
)

// Read parses OFX from r. It returns an error if no <STMTTRN> blocks are found
// (most often a wrong-format upload).
func (OFX) Read(r io.Reader) ([]app.StatementTxn, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	blocks := stmtTrnRe.FindAllStringSubmatch(string(raw), -1)
	if len(blocks) == 0 {
		return nil, fmt.Errorf("no transactions found (is this an OFX file?)")
	}

	out := make([]app.StatementTxn, 0, len(blocks))
	for _, b := range blocks {
		body := b[1]

		dt := field(dtPostedRe, body)
		date, err := parseOFXDate(dt)
		if err != nil {
			return nil, err
		}
		amtStr := field(trnAmtRe, body)
		amt, err := domain.FromDecimalString(amtStr)
		if err != nil {
			return nil, fmt.Errorf("invalid OFX amount %q: %w", amtStr, err)
		}
		out = append(out, app.StatementTxn{
			Date:   date,
			Amount: amt,
			Memo:   joinNonEmpty(field(nameRe, body), field(memoRe, body)),
			FITID:  field(fitIDRe, body),
		})
	}
	return out, nil
}

func field(re *regexp.Regexp, body string) string {
	if m := re.FindStringSubmatch(body); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// parseOFXDate reads an OFX DTPOSTED. The format is YYYYMMDD optionally followed
// by HHMMSS and a [tz] suffix; only the leading date is needed.
func parseOFXDate(s string) (time.Time, error) {
	if len(s) < 8 {
		return time.Time{}, fmt.Errorf("unrecognised OFX date %q", s)
	}
	t, err := time.Parse("20060102", s[:8])
	if err != nil {
		return time.Time{}, fmt.Errorf("unrecognised OFX date %q: %w", s, err)
	}
	return t, nil
}
