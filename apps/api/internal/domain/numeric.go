// Package domain holds OpenLedger's GnuCash-derived accounting kernel. It is
// pure Go with no database or HTTP dependencies so the accounting invariants
// can be tested in isolation.
package domain

import (
	"fmt"
	"math/big"
	"strings"
)

// GncNumeric is an exact rational monetary amount, mirroring GnuCash's
// numerator/denominator representation (the *_num / *_denom column pairs). It
// is backed by math/big.Rat, so all arithmetic is exact and reduced; floating
// point is never used anywhere in the money path.
//
// The zero value is a valid 0.
type GncNumeric struct {
	r *big.Rat
}

// rat returns the underlying rational, tolerating the zero value.
func (n GncNumeric) rat() *big.Rat {
	if n.r == nil {
		return new(big.Rat)
	}
	return n.r
}

// Zero returns a GncNumeric equal to 0.
func Zero() GncNumeric { return GncNumeric{r: new(big.Rat)} }

// FromNumDenom builds a GncNumeric from a GnuCash-style numerator/denominator
// pair, as persisted in *_num / *_denom columns. denom must be non-zero.
func FromNumDenom(num, denom int64) (GncNumeric, error) {
	if denom == 0 {
		return GncNumeric{}, fmt.Errorf("gncnumeric: zero denominator")
	}
	return GncNumeric{r: new(big.Rat).SetFrac(big.NewInt(num), big.NewInt(denom))}, nil
}

// MustFromNumDenom is like FromNumDenom but panics on a zero denominator.
// Intended for tests and compile-time constants.
func MustFromNumDenom(num, denom int64) GncNumeric {
	n, err := FromNumDenom(num, denom)
	if err != nil {
		panic(err)
	}
	return n
}

// FromDecimalString parses a decimal ("12.50") or fraction ("3/4") string.
func FromDecimalString(s string) (GncNumeric, error) {
	r, ok := new(big.Rat).SetString(strings.TrimSpace(s))
	if !ok {
		return GncNumeric{}, fmt.Errorf("gncnumeric: invalid amount %q", s)
	}
	return GncNumeric{r: r}, nil
}

// Add returns n + other.
func (n GncNumeric) Add(other GncNumeric) GncNumeric {
	return GncNumeric{r: new(big.Rat).Add(n.rat(), other.rat())}
}

// Sub returns n - other.
func (n GncNumeric) Sub(other GncNumeric) GncNumeric {
	return GncNumeric{r: new(big.Rat).Sub(n.rat(), other.rat())}
}

// Mul returns n * other.
func (n GncNumeric) Mul(other GncNumeric) GncNumeric {
	return GncNumeric{r: new(big.Rat).Mul(n.rat(), other.rat())}
}

// Neg returns -n.
func (n GncNumeric) Neg() GncNumeric {
	return GncNumeric{r: new(big.Rat).Neg(n.rat())}
}

// Div returns n / other as an exact rational. It errors when other is zero.
// Used to apportion a lot's cost basis across a partial sale.
func (n GncNumeric) Div(other GncNumeric) (GncNumeric, error) {
	if other.IsZero() {
		return GncNumeric{}, fmt.Errorf("gncnumeric: division by zero")
	}
	return GncNumeric{r: new(big.Rat).Quo(n.rat(), other.rat())}, nil
}

// Cmp compares n and other, returning -1, 0, or +1.
func (n GncNumeric) Cmp(other GncNumeric) int { return n.rat().Cmp(other.rat()) }

// Equal reports whether n and other are the same value.
func (n GncNumeric) Equal(other GncNumeric) bool { return n.Cmp(other) == 0 }

// Sign returns -1, 0, or +1.
func (n GncNumeric) Sign() int { return n.rat().Sign() }

// IsZero reports whether n is exactly zero.
func (n GncNumeric) IsZero() bool { return n.rat().Sign() == 0 }

// NumDenom returns the reduced numerator and denominator, suitable for storing
// back into *_num / *_denom columns. It errors if either does not fit in int64.
func (n GncNumeric) NumDenom() (num, denom int64, err error) {
	r := n.rat()
	if !r.Num().IsInt64() || !r.Denom().IsInt64() {
		return 0, 0, fmt.Errorf("gncnumeric: value %s does not fit in int64 num/denom", r.RatString())
	}
	return r.Num().Int64(), r.Denom().Int64(), nil
}

// AtDenom expresses the amount with the given denominator (e.g. a commodity's
// smallest-currency-unit fraction such as 100 for cents) and returns the
// resulting numerator. It errors if the amount is not exact at that
// denominator, so rounding is always an explicit decision elsewhere.
func (n GncNumeric) AtDenom(denom int64) (int64, error) {
	if denom == 0 {
		return 0, fmt.Errorf("gncnumeric: zero denominator")
	}
	scaled := new(big.Rat).Mul(n.rat(), new(big.Rat).SetInt64(denom))
	if !scaled.IsInt() {
		return 0, fmt.Errorf("gncnumeric: %s is not exact at denominator %d", n.String(), denom)
	}
	v := scaled.Num()
	if !v.IsInt64() {
		return 0, fmt.Errorf("gncnumeric: value overflows int64 at denominator %d", denom)
	}
	return v.Int64(), nil
}

// String renders the value as a reduced rational ("5", "3/2").
func (n GncNumeric) String() string { return n.rat().RatString() }

// DecimalString renders the value rounded to prec decimal places.
func (n GncNumeric) DecimalString(prec int) string { return n.rat().FloatString(prec) }

// Sum returns the total of all amounts (0 for an empty list).
func Sum(amounts ...GncNumeric) GncNumeric {
	total := new(big.Rat)
	for _, a := range amounts {
		total.Add(total, a.rat())
	}
	return GncNumeric{r: total}
}
