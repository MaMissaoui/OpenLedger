package domain

import "testing"

func TestAddSubExact(t *testing.T) {
	// 0.10 + 0.20 must equal exactly 0.30 (the classic float trap).
	a := MustFromNumDenom(10, 100)
	b := MustFromNumDenom(20, 100)
	got := a.Add(b)
	want := MustFromNumDenom(30, 100)
	if !got.Equal(want) {
		t.Fatalf("0.10 + 0.20 = %s, want %s", got, want)
	}
	if back := got.Sub(b); !back.Equal(a) {
		t.Fatalf("0.30 - 0.20 = %s, want %s", back, a)
	}
}

func TestFromDecimalString(t *testing.T) {
	cases := map[string]GncNumeric{
		"12.50": MustFromNumDenom(25, 2),
		"3/4":   MustFromNumDenom(3, 4),
		"-5":    MustFromNumDenom(-5, 1),
	}
	for in, want := range cases {
		got, err := FromDecimalString(in)
		if err != nil {
			t.Fatalf("FromDecimalString(%q) error: %v", in, err)
		}
		if !got.Equal(want) {
			t.Errorf("FromDecimalString(%q) = %s, want %s", in, got, want)
		}
	}
	if _, err := FromDecimalString("not-a-number"); err == nil {
		t.Error("expected error for invalid input")
	}
}

func TestNumDenomReduced(t *testing.T) {
	// 50/100 reduces to 1/2.
	num, denom, err := MustFromNumDenom(50, 100).NumDenom()
	if err != nil {
		t.Fatal(err)
	}
	if num != 1 || denom != 2 {
		t.Fatalf("NumDenom() = %d/%d, want 1/2", num, denom)
	}
}

func TestAtDenom(t *testing.T) {
	// 12.50 expressed in cents (denom 100) is 1250.
	got, err := MustFromNumDenom(25, 2).AtDenom(100)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1250 {
		t.Fatalf("AtDenom(100) = %d, want 1250", got)
	}
	// 1/3 is not exact in cents and must error rather than silently round.
	if _, err := MustFromNumDenom(1, 3).AtDenom(100); err == nil {
		t.Error("expected inexactness error for 1/3 at denom 100")
	}
}

func TestZeroValueIsUsable(t *testing.T) {
	var z GncNumeric // zero value, no constructor
	if !z.IsZero() {
		t.Fatal("zero-value GncNumeric should be zero")
	}
	if got := z.Add(MustFromNumDenom(5, 1)); !got.Equal(MustFromNumDenom(5, 1)) {
		t.Fatalf("0 + 5 = %s, want 5", got)
	}
}

func TestSum(t *testing.T) {
	got := Sum(
		MustFromNumDenom(100, 100),
		MustFromNumDenom(-40, 100),
		MustFromNumDenom(-60, 100),
	)
	if !got.IsZero() {
		t.Fatalf("Sum = %s, want 0", got)
	}
}
