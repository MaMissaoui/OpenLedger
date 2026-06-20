// Package bankimport provides StatementReader implementations that parse bank
// statement files (OFX, QIF) into app.StatementTxn values for import into an
// existing account.
package bankimport

import "strings"

// joinNonEmpty joins the non-empty parts with " — ", giving a single readable
// description from a statement line's payee/name and memo fields.
func joinNonEmpty(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			kept = append(kept, s)
		}
	}
	return strings.Join(kept, " — ")
}
