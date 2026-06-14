package domain

import (
	"errors"
	"testing"
)

func TestDivExact(t *testing.T) {
	// 1500 / 10 = 150 exactly.
	got, err := MustFromNumDenom(1500, 1).Div(MustFromNumDenom(10, 1))
	if err != nil {
		t.Fatalf("div: %v", err)
	}
	if want := MustFromNumDenom(150, 1); !got.Equal(want) {
		t.Errorf("1500/10 = %s, want %s", got, want)
	}
	// Non-terminating in decimal but exact as a rational: 100/3.
	third, err := MustFromNumDenom(100, 1).Div(MustFromNumDenom(3, 1))
	if err != nil {
		t.Fatalf("div: %v", err)
	}
	if !third.Mul(MustFromNumDenom(3, 1)).Equal(MustFromNumDenom(100, 1)) {
		t.Errorf("(100/3)*3 should be exactly 100, got %s", third.Mul(MustFromNumDenom(3, 1)))
	}
}

func TestDivByZero(t *testing.T) {
	if _, err := MustFromNumDenom(5, 1).Div(Zero()); err == nil {
		t.Fatal("dividing by zero should error")
	}
}

// lot is a shorthand for an open lot of whole shares at a whole-dollar cost.
func lot(guid string, shares, cost int64) OpenLot {
	return OpenLot{GUID: guid, Remaining: MustFromNumDenom(shares, 1), Cost: MustFromNumDenom(cost, 1)}
}

func TestMatchFIFOSingleLotPartial(t *testing.T) {
	// One lot of 10 shares costing $1,500 ($150/share). Sell 4.
	res, err := MatchFIFO([]OpenLot{lot("L1", 10, 1500)}, MustFromNumDenom(4, 1))
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if len(res.Consumptions) != 1 {
		t.Fatalf("consumptions = %d, want 1", len(res.Consumptions))
	}
	c := res.Consumptions[0]
	if c.ClosesLot {
		t.Error("a 4-of-10 sale must not close the lot")
	}
	// 4 × $150 = $600 cost removed.
	if want := MustFromNumDenom(600, 1); !c.Cost.Equal(want) {
		t.Errorf("cost = %s, want %s", c.Cost, want)
	}
	if !res.TotalCost.Equal(MustFromNumDenom(600, 1)) {
		t.Errorf("total cost = %s, want 600", res.TotalCost)
	}
}

func TestMatchFIFOSpansLotsOldestFirst(t *testing.T) {
	// Two lots: L1 = 10 @ $1,500 (oldest), L2 = 5 @ $1,000. Sell 12 → all of L1
	// plus 2 of L2.
	lots := []OpenLot{lot("L1", 10, 1500), lot("L2", 5, 1000)}
	res, err := MatchFIFO(lots, MustFromNumDenom(12, 1))
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if len(res.Consumptions) != 2 {
		t.Fatalf("consumptions = %d, want 2", len(res.Consumptions))
	}
	if res.Consumptions[0].LotGUID != "L1" || !res.Consumptions[0].ClosesLot {
		t.Errorf("first consumption should fully close L1: %+v", res.Consumptions[0])
	}
	if res.Consumptions[1].LotGUID != "L2" || res.Consumptions[1].ClosesLot {
		t.Errorf("second consumption should partially take L2: %+v", res.Consumptions[1])
	}
	// L1: full $1,500. L2: 2 of 5 = $400. Total $1,900.
	if want := MustFromNumDenom(1900, 1); !res.TotalCost.Equal(want) {
		t.Errorf("total cost = %s, want %s", res.TotalCost, want)
	}
}

func TestMatchFIFOExactWholeLot(t *testing.T) {
	res, err := MatchFIFO([]OpenLot{lot("L1", 10, 1500)}, MustFromNumDenom(10, 1))
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if len(res.Consumptions) != 1 || !res.Consumptions[0].ClosesLot {
		t.Fatalf("selling the whole lot should close it: %+v", res.Consumptions)
	}
	if !res.TotalCost.Equal(MustFromNumDenom(1500, 1)) {
		t.Errorf("total cost = %s, want 1500", res.TotalCost)
	}
}

func TestMatchFIFOInsufficientShares(t *testing.T) {
	_, err := MatchFIFO([]OpenLot{lot("L1", 10, 1500)}, MustFromNumDenom(11, 1))
	if !errors.Is(err, ErrInsufficientShares) {
		t.Fatalf("err = %v, want ErrInsufficientShares", err)
	}
}

func TestMatchFIFONonPositive(t *testing.T) {
	if _, err := MatchFIFO([]OpenLot{lot("L1", 10, 1500)}, Zero()); err == nil {
		t.Fatal("zero sale quantity should error")
	}
}
