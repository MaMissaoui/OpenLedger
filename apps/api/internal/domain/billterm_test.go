package domain

import "testing"

func TestBillTermDueDate(t *testing.T) {
	t.Run("days term adds net days to the invoice date", func(t *testing.T) {
		term := BillTerm{Type: BillTermDays, DueDays: 30}
		got := term.DueDate(date(2024, 1, 15))
		if !got.Equal(date(2024, 2, 14)) {
			t.Errorf("got %v, want 2024-02-14", got)
		}
	})

	t.Run("proximo term on or before cutoff is due the due-day of next month", func(t *testing.T) {
		term := BillTerm{Type: BillTermProximo, DueDays: 15, Cutoff: 25}
		got := term.DueDate(date(2024, 1, 10))
		if !got.Equal(date(2024, 2, 15)) {
			t.Errorf("got %v, want 2024-02-15", got)
		}
	})

	t.Run("proximo term after cutoff rolls to the month after next", func(t *testing.T) {
		term := BillTerm{Type: BillTermProximo, DueDays: 15, Cutoff: 25}
		got := term.DueDate(date(2024, 1, 26))
		if !got.Equal(date(2024, 3, 15)) {
			t.Errorf("got %v, want 2024-03-15", got)
		}
	})

	t.Run("proximo clamps the due day to the last day of a short month", func(t *testing.T) {
		term := BillTerm{Type: BillTermProximo, DueDays: 31, Cutoff: 25}
		got := term.DueDate(date(2024, 1, 10)) // due in February (leap year)
		if !got.Equal(date(2024, 2, 29)) {
			t.Errorf("got %v, want 2024-02-29", got)
		}
	})
}

func TestBillTermDiscountDate(t *testing.T) {
	t.Run("discount date is the discount-days window when a discount is set", func(t *testing.T) {
		term := BillTerm{
			Type:         BillTermDays,
			DueDays:      30,
			DiscountDays: 10,
			Discount:     MustFromNumDenom(2, 100), // 2% early-payment discount
		}
		got, ok := term.DiscountDate(date(2024, 1, 15))
		if !ok {
			t.Fatal("expected a discount date, got none")
		}
		if !got.Equal(date(2024, 1, 25)) {
			t.Errorf("got %v, want 2024-01-25", got)
		}
	})

	t.Run("no discount date when the discount is zero", func(t *testing.T) {
		term := BillTerm{Type: BillTermDays, DueDays: 30, DiscountDays: 10}
		if _, ok := term.DiscountDate(date(2024, 1, 15)); ok {
			t.Error("expected no discount date when discount is zero")
		}
	})
}

func TestBillTermValidate(t *testing.T) {
	t.Run("a net-days term is valid", func(t *testing.T) {
		term := BillTerm{Type: BillTermDays, DueDays: 30}
		if err := term.Validate(); err != nil {
			t.Errorf("expected valid, got %v", err)
		}
	})

	t.Run("an unknown type is rejected", func(t *testing.T) {
		term := BillTerm{Type: "fortnightly", DueDays: 30}
		if err := term.Validate(); err == nil {
			t.Error("expected an error for an unknown term type")
		}
	})

	t.Run("negative due days are rejected", func(t *testing.T) {
		term := BillTerm{Type: BillTermDays, DueDays: -1}
		if err := term.Validate(); err == nil {
			t.Error("expected an error for negative due days")
		}
	})

	t.Run("a proximo due day outside 1..31 is rejected", func(t *testing.T) {
		term := BillTerm{Type: BillTermProximo, DueDays: 32, Cutoff: 25}
		if err := term.Validate(); err == nil {
			t.Error("expected an error for an out-of-range proximo due day")
		}
	})
}
