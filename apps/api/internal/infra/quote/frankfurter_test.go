package quote

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestFrankfurterFetchRate(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		// A rate with trailing precision a float64 round-trips imperfectly; the
		// provider must parse the decimal text exactly.
		_, _ = io.WriteString(w, `{"amount":1.0,"base":"USD","date":"2024-06-19","rates":{"EUR":0.9321}}`)
	}))
	defer srv.Close()

	f := NewFrankfurter(srv.Client(), srv.URL)
	q, err := f.FetchRate(context.Background(), "USD", "EUR")
	if err != nil {
		t.Fatalf("FetchRate: %v", err)
	}
	if gotQuery.Get("base") != "USD" || gotQuery.Get("symbols") != "EUR" {
		t.Errorf("request query = %v, want base=USD symbols=EUR", gotQuery)
	}
	if got := q.Rate.DecimalString(4); got != "0.9321" {
		t.Errorf("rate = %s, want 0.9321", got)
	}
	if got := q.Date.Format("2006-01-02"); got != "2024-06-19" {
		t.Errorf("date = %s, want 2024-06-19", got)
	}
	if f.Name() != "frankfurter" {
		t.Errorf("Name() = %q, want frankfurter", f.Name())
	}
}

func TestFrankfurterFetchRateMissingRate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"base":"USD","date":"2024-06-19","rates":{}}`)
	}))
	defer srv.Close()

	f := NewFrankfurter(srv.Client(), srv.URL)
	if _, err := f.FetchRate(context.Background(), "USD", "EUR"); err == nil {
		t.Fatal("expected an error when the response has no matching rate")
	}
}

func TestFrankfurterFetchRateNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := NewFrankfurter(srv.Client(), srv.URL)
	if _, err := f.FetchRate(context.Background(), "USD", "EUR"); err == nil {
		t.Fatal("expected an error on a non-200 upstream response")
	}
}
