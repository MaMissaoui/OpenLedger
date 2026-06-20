package domain

import "testing"

func TestResolveDueDate(t *testing.T) {
	invoiceDate := date(2024, 1, 15)

	t.Run("an explicit due date always wins over a term", func(t *testing.T) {
		explicit := date(2024, 3, 1)
		term := &BillTerm{Type: BillTermDays, DueDays: 30}
		got := ResolveDueDate(&explicit, term, invoiceDate)
		if got == nil || !got.Equal(explicit) {
			t.Errorf("got %v, want explicit %v", got, explicit)
		}
	})

	t.Run("with no explicit date the due date derives from the term", func(t *testing.T) {
		term := &BillTerm{Type: BillTermDays, DueDays: 30}
		got := ResolveDueDate(nil, term, invoiceDate)
		if got == nil || !got.Equal(date(2024, 2, 14)) {
			t.Errorf("got %v, want 2024-02-14", got)
		}
	})

	t.Run("with neither explicit date nor term the due date is unset", func(t *testing.T) {
		if got := ResolveDueDate(nil, nil, invoiceDate); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})
}
