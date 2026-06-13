package domain

// NamespaceCurrency is the GnuCash commodity namespace for ISO currencies.
// Tradable securities use their exchange (e.g. "NASDAQ") instead.
const NamespaceCurrency = "CURRENCY"

// Commodity is a currency or a tradable instrument. Fraction is the smallest
// representable unit's denominator (100 for a currency with cents), and is the
// denominator every amount in this commodity is stored at. It corresponds to a
// row in GnuCash's commodities table.
type Commodity struct {
	GUID      string
	Namespace string
	Mnemonic  string
	Fullname  string
	Fraction  int64
}
