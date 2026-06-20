package domain

import "testing"

func TestAggregateTax(t *testing.T) {
	vat := &TaxTable{Entries: []TaxTableEntry{
		{AccountGUID: "vat-acc", Type: TaxPercentage, Amount: MustFromNumDenom(10, 1)},
	}}

	t.Run("lines sharing a tax account merge into one charge and a grand total", func(t *testing.T) {
		lines := []TaxableLine{
			{Base: MustFromNumDenom(100, 1), Table: vat}, // 10
			{Base: MustFromNumDenom(50, 1), Table: vat},  // 5
		}
		charges, total := AggregateTax(lines)
		if len(charges) != 1 {
			t.Fatalf("got %d charges, want 1 (merged by account): %+v", len(charges), charges)
		}
		if charges[0].AccountGUID != "vat-acc" {
			t.Errorf("account: got %q, want vat-acc", charges[0].AccountGUID)
		}
		if want := MustFromNumDenom(15, 1); charges[0].Amount.Cmp(want) != 0 {
			t.Errorf("merged charge: got %v, want %v", charges[0].Amount, want)
		}
		if want := MustFromNumDenom(15, 1); total.Cmp(want) != 0 {
			t.Errorf("total: got %v, want %v", total, want)
		}
	})

	t.Run("a line with no table contributes no tax", func(t *testing.T) {
		lines := []TaxableLine{
			{Base: MustFromNumDenom(100, 1), Table: nil},
		}
		charges, total := AggregateTax(lines)
		if len(charges) != 0 || !total.IsZero() {
			t.Errorf("expected no tax, got charges=%+v total=%v", charges, total)
		}
	})
}
