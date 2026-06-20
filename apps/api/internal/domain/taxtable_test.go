package domain

import "testing"

func TestTaxTableComputeTax(t *testing.T) {
	t.Run("a single percentage entry taxes the base at that rate", func(t *testing.T) {
		tt := TaxTable{Entries: []TaxTableEntry{
			{AccountGUID: "vat-acc", Type: TaxPercentage, Amount: MustFromNumDenom(10, 1)}, // 10%
		}}
		charges := tt.ComputeTax(MustFromNumDenom(100, 1))
		if len(charges) != 1 {
			t.Fatalf("got %d charges, want 1", len(charges))
		}
		if charges[0].AccountGUID != "vat-acc" {
			t.Errorf("account: got %q, want vat-acc", charges[0].AccountGUID)
		}
		if want := MustFromNumDenom(10, 1); charges[0].Amount.Cmp(want) != 0 {
			t.Errorf("amount: got %v, want %v", charges[0].Amount, want)
		}
	})

	t.Run("a value entry charges a flat amount regardless of the base", func(t *testing.T) {
		tt := TaxTable{Entries: []TaxTableEntry{
			{AccountGUID: "duty-acc", Type: TaxValue, Amount: MustFromNumDenom(5, 1)},
		}}
		charges := tt.ComputeTax(MustFromNumDenom(100, 1))
		if len(charges) != 1 || charges[0].AccountGUID != "duty-acc" {
			t.Fatalf("unexpected charges %+v", charges)
		}
		if want := MustFromNumDenom(5, 1); charges[0].Amount.Cmp(want) != 0 {
			t.Errorf("amount: got %v, want %v", charges[0].Amount, want)
		}
	})
}

func TestTaxTableTotalTax(t *testing.T) {
	t.Run("total tax sums every entry over the base", func(t *testing.T) {
		tt := TaxTable{Entries: []TaxTableEntry{
			{AccountGUID: "vat-acc", Type: TaxPercentage, Amount: MustFromNumDenom(10, 1)}, // 10
			{AccountGUID: "duty-acc", Type: TaxValue, Amount: MustFromNumDenom(5, 1)},      // 5
		}}
		total := tt.TotalTax(MustFromNumDenom(100, 1))
		if want := MustFromNumDenom(15, 1); total.Cmp(want) != 0 {
			t.Errorf("total: got %v, want %v", total, want)
		}
	})
}

func TestTaxTableValidate(t *testing.T) {
	valid := TaxTableEntry{AccountGUID: "acc", Type: TaxPercentage, Amount: MustFromNumDenom(10, 1)}

	t.Run("a table with at least one well-formed entry is valid", func(t *testing.T) {
		tt := TaxTable{Name: "VAT", Entries: []TaxTableEntry{valid}}
		if err := tt.Validate(); err != nil {
			t.Errorf("expected valid, got %v", err)
		}
	})

	t.Run("a table with no entries is rejected", func(t *testing.T) {
		tt := TaxTable{Name: "VAT"}
		if err := tt.Validate(); err == nil {
			t.Error("expected an error for a table with no entries")
		}
	})

	t.Run("an entry with an unknown type is rejected", func(t *testing.T) {
		tt := TaxTable{Name: "VAT", Entries: []TaxTableEntry{{AccountGUID: "acc", Type: "flat", Amount: MustFromNumDenom(1, 1)}}}
		if err := tt.Validate(); err == nil {
			t.Error("expected an error for an unknown entry type")
		}
	})

	t.Run("an entry with no account is rejected", func(t *testing.T) {
		tt := TaxTable{Name: "VAT", Entries: []TaxTableEntry{{Type: TaxPercentage, Amount: MustFromNumDenom(10, 1)}}}
		if err := tt.Validate(); err == nil {
			t.Error("expected an error for an entry with no account")
		}
	})

	t.Run("a negative amount is rejected", func(t *testing.T) {
		tt := TaxTable{Name: "VAT", Entries: []TaxTableEntry{{AccountGUID: "acc", Type: TaxPercentage, Amount: MustFromNumDenom(-5, 1)}}}
		if err := tt.Validate(); err == nil {
			t.Error("expected an error for a negative amount")
		}
	})
}
