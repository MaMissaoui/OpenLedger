package domain

import (
	"testing"
	"time"
)

func jan(y, m, d int) time.Time {
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
}

func monthlyBudget() Budget {
	return Budget{
		PeriodType: BudgetMonthly,
		NumPeriods: 12,
		StartDate:  jan(2024, 1, 1),
	}
}

func TestPeriodStart(t *testing.T) {
	b := monthlyBudget()
	cases := []struct {
		i    int
		want time.Time
	}{
		{0, jan(2024, 1, 1)},
		{1, jan(2024, 2, 1)},
		{11, jan(2024, 12, 1)},
	}
	for _, c := range cases {
		got := b.PeriodStart(c.i)
		if !got.Equal(c.want) {
			t.Errorf("PeriodStart(%d) = %v, want %v", c.i, got, c.want)
		}
	}
}

func TestPeriodEnd(t *testing.T) {
	b := monthlyBudget()
	// Jan end should be just before Feb 1.
	end := b.PeriodEnd(0)
	febStart := b.PeriodStart(1)
	if !end.Before(febStart) || end.Add(time.Second) != febStart {
		t.Errorf("PeriodEnd(0) = %v, want one second before %v", end, febStart)
	}
}

func TestPeriodIndex(t *testing.T) {
	b := monthlyBudget()
	cases := []struct {
		d    time.Time
		want int
	}{
		{jan(2024, 1, 1), 0},
		{jan(2024, 1, 31), 0},
		{jan(2024, 2, 1), 1},
		{jan(2024, 12, 31), 11},
		{jan(2023, 12, 31), -1}, // before start
		{jan(2025, 1, 1), -1},   // after last period
	}
	for _, c := range cases {
		got := b.PeriodIndex(c.d)
		if got != c.want {
			t.Errorf("PeriodIndex(%v) = %d, want %d", c.d, got, c.want)
		}
	}
}

func TestQuarterlyPeriodIndex(t *testing.T) {
	b := Budget{PeriodType: BudgetQuarterly, NumPeriods: 4, StartDate: jan(2024, 1, 1)}
	if b.PeriodIndex(jan(2024, 3, 31)) != 0 {
		t.Error("Q1 Mar should be period 0")
	}
	if b.PeriodIndex(jan(2024, 4, 1)) != 1 {
		t.Error("Q2 Apr should be period 1")
	}
	if b.PeriodIndex(jan(2025, 1, 1)) != -1 {
		t.Error("Past end should be -1")
	}
}

func TestYearlyPeriodLabel(t *testing.T) {
	b := Budget{PeriodType: BudgetYearly, NumPeriods: 3, StartDate: jan(2024, 1, 1)}
	if b.PeriodLabel(0) != "2024" {
		t.Errorf("got %q, want 2024", b.PeriodLabel(0))
	}
	if b.PeriodLabel(1) != "2025" {
		t.Errorf("got %q, want 2025", b.PeriodLabel(1))
	}
}
