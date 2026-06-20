// Package quote provides QuoteProvider implementations that fetch live exchange
// rates — the pluggable replacement for GnuCash's Finance::Quote.
package quote

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// DefaultFrankfurterURL is the Frankfurter "latest rates" endpoint (ECB
// reference rates; free, no API key).
const DefaultFrankfurterURL = "https://api.frankfurter.dev/v1/latest"

// Frankfurter fetches currency exchange rates from the Frankfurter service. It
// implements app.QuoteProvider for currency commodities only (no equities).
type Frankfurter struct {
	client  *http.Client
	baseURL string
}

// NewFrankfurter builds a provider. A nil client gets a 10s-timeout default; an
// empty baseURL gets DefaultFrankfurterURL (both overridable for tests).
func NewFrankfurter(client *http.Client, baseURL string) *Frankfurter {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if baseURL == "" {
		baseURL = DefaultFrankfurterURL
	}
	return &Frankfurter{client: client, baseURL: baseURL}
}

// Name identifies the provider in a stored price's Source ("quote:frankfurter").
func (f *Frankfurter) Name() string { return "frankfurter" }

// FetchRate returns how many units of quote one unit of base buys, as of the
// provider's latest reference date. The rate is parsed from the response's
// decimal text into an exact GncNumeric — never through a float64.
func (f *Frankfurter) FetchRate(ctx context.Context, base, quote string) (app.Quote, error) {
	u, err := url.Parse(f.baseURL)
	if err != nil {
		return app.Quote{}, fmt.Errorf("invalid base URL: %w", err)
	}
	q := u.Query()
	q.Set("base", base)
	q.Set("symbols", quote)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return app.Quote{}, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return app.Quote{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return app.Quote{}, fmt.Errorf("frankfurter returned %s", resp.Status)
	}

	// UseNumber keeps each rate as its original decimal text so we can build an
	// exact rational from it rather than a lossy float.
	var payload struct {
		Date  string                 `json:"date"`
		Rates map[string]json.Number `json:"rates"`
	}
	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	if err := dec.Decode(&payload); err != nil {
		return app.Quote{}, fmt.Errorf("decode response: %w", err)
	}

	raw, ok := payload.Rates[quote]
	if !ok {
		return app.Quote{}, fmt.Errorf("no %s/%s rate in response", base, quote)
	}
	rate, err := domain.FromDecimalString(raw.String())
	if err != nil {
		return app.Quote{}, fmt.Errorf("parse rate %q: %w", raw.String(), err)
	}

	date, err := time.Parse("2006-01-02", payload.Date)
	if err != nil {
		// Provider omitted or garbled the date; fall back to now.
		date = time.Now()
	}
	return app.Quote{Rate: rate, Date: date}, nil
}
