package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/openledger/openledger/apps/api/internal/app"
	"github.com/openledger/openledger/apps/api/internal/domain"
)

// fakeScheduleRepo satisfies both ScheduleRepository and TransactionRepository.
type fakeScheduleRepo struct {
	schedules map[string]domain.ScheduledTransaction
	inserted  []domain.Transaction
}

func newFakeSched() *fakeScheduleRepo {
	return &fakeScheduleRepo{schedules: make(map[string]domain.ScheduledTransaction)}
}

func (f *fakeScheduleRepo) InsertTransaction(_ context.Context, tx domain.Transaction, _ app.AuditActor) error {
	f.inserted = append(f.inserted, tx)
	return nil
}
func (f *fakeScheduleRepo) UpdateTransaction(_ context.Context, _ domain.Transaction, _ app.AuditActor) error {
	return nil
}
func (f *fakeScheduleRepo) DeleteTransaction(_ context.Context, _ string, _ app.AuditActor) error {
	return nil
}
func (f *fakeScheduleRepo) TransactionAccountGUIDs(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}
func (f *fakeScheduleRepo) CreateScheduledTransaction(_ context.Context, s domain.ScheduledTransaction) (domain.ScheduledTransaction, error) {
	f.schedules[s.GUID] = s
	return s, nil
}
func (f *fakeScheduleRepo) ListScheduledTransactions(_ context.Context, bookGUID string) ([]domain.ScheduledTransaction, error) {
	var out []domain.ScheduledTransaction
	for _, s := range f.schedules {
		if s.BookGUID == bookGUID {
			out = append(out, s)
		}
	}
	return out, nil
}
func (f *fakeScheduleRepo) GetScheduledTransaction(_ context.Context, guid string) (domain.ScheduledTransaction, error) {
	s, ok := f.schedules[guid]
	if !ok {
		return domain.ScheduledTransaction{}, app.ErrScheduleNotFound
	}
	return s, nil
}
func (f *fakeScheduleRepo) UpdateScheduledTransaction(_ context.Context, s domain.ScheduledTransaction) (domain.ScheduledTransaction, error) {
	if _, ok := f.schedules[s.GUID]; !ok {
		return domain.ScheduledTransaction{}, app.ErrScheduleNotFound
	}
	f.schedules[s.GUID] = s
	return s, nil
}
func (f *fakeScheduleRepo) DeleteScheduledTransaction(_ context.Context, guid string) error {
	if _, ok := f.schedules[guid]; !ok {
		return app.ErrScheduleNotFound
	}
	delete(f.schedules, guid)
	return nil
}
func (f *fakeScheduleRepo) BookGUIDForSchedule(_ context.Context, guid string) (string, error) {
	s, ok := f.schedules[guid]
	if !ok {
		return "", app.ErrScheduleNotFound
	}
	return s.BookGUID, nil
}
func (f *fakeScheduleRepo) MarkSchedulePosted(_ context.Context, guid string, date time.Time) error {
	s, ok := f.schedules[guid]
	if !ok {
		return app.ErrScheduleNotFound
	}
	s.LastPostedDate = date
	f.schedules[guid] = s
	return nil
}

func newSchedHandler(sr *fakeScheduleRepo) http.Handler {
	posting := app.NewPostingService(sr)
	return authedServer(Services{Schedule: app.NewScheduleService(sr, posting)})
}

func schedReq(h http.Handler, method, path string, body string) *httptest.ResponseRecorder {
	var reqBody *strings.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	} else {
		reqBody = strings.NewReader("")
	}
	req := withAuth(httptest.NewRequest(method, path, reqBody))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHandleListScheduledTransactions(t *testing.T) {
	sr := newFakeSched()
	sr.schedules["s1"] = domain.ScheduledTransaction{
		GUID: "s1", BookGUID: "book-1", Name: "Monthly Rent",
		Enabled: true, CurrencyGUID: "usd", Period: domain.PeriodMonthly, Every: 1,
		StartDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Splits: []domain.ScheduledSplit{
			{GUID: "ss1", AccountGUID: "a1", Value: domain.MustFromNumDenom(200000, 100)},
			{GUID: "ss2", AccountGUID: "a2", Value: domain.MustFromNumDenom(-200000, 100)},
		},
	}

	rec := schedReq(newSchedHandler(sr), "GET", "/api/v1/books/book-1/scheduled-transactions", "")
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
}

func TestHandleCreateScheduledTransaction(t *testing.T) {
	sr := newFakeSched()
	h := newSchedHandler(sr)

	body := `{"name":"Monthly Rent","enabled":true,"currencyGuid":"usd","period":"monthly","every":1,"startDate":"2024-01-01","splits":[{"accountGuid":"a1","memo":"","value":{"num":200000,"denom":100}},{"accountGuid":"a2","memo":"","value":{"num":-200000,"denom":100}}]}`
	rec := schedReq(h, "POST", "/api/v1/books/book-1/scheduled-transactions", body)
	if rec.Code != 201 {
		t.Fatalf("status = %d, want 201; body: %s", rec.Code, rec.Body)
	}
}

func TestHandleCreateScheduledTransactionUnbalanced(t *testing.T) {
	sr := newFakeSched()
	h := newSchedHandler(sr)

	body := `{"name":"Bad","enabled":true,"currencyGuid":"usd","period":"monthly","every":1,"startDate":"2024-01-01","splits":[{"accountGuid":"a1","value":{"num":200000,"denom":100}},{"accountGuid":"a2","value":{"num":-100000,"denom":100}}]}`
	rec := schedReq(h, "POST", "/api/v1/books/book-1/scheduled-transactions", body)
	if rec.Code != 422 {
		t.Fatalf("status = %d, want 422; body: %s", rec.Code, rec.Body)
	}
}

func TestHandleDeleteScheduledTransaction(t *testing.T) {
	sr := newFakeSched()
	sr.schedules["s1"] = domain.ScheduledTransaction{
		GUID: "s1", BookGUID: "book-1", Name: "Rent",
		Enabled: true, CurrencyGUID: "usd", Period: domain.PeriodMonthly, Every: 1,
		StartDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Splits: []domain.ScheduledSplit{
			{GUID: "ss1", AccountGUID: "a1", Value: domain.MustFromNumDenom(100, 1)},
			{GUID: "ss2", AccountGUID: "a2", Value: domain.MustFromNumDenom(-100, 1)},
		},
	}

	rec := schedReq(newSchedHandler(sr), "DELETE", "/api/v1/scheduled-transactions/s1", "")
	if rec.Code != 204 {
		t.Fatalf("status = %d, want 204; body: %s", rec.Code, rec.Body)
	}
}

func TestHandlePostDueSchedules(t *testing.T) {
	sr := newFakeSched()
	sr.schedules["s1"] = domain.ScheduledTransaction{
		GUID: "s1", BookGUID: "book-1", Name: "Salary",
		Enabled: true, CurrencyGUID: "usd", Period: domain.PeriodMonthly, Every: 1,
		StartDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Splits: []domain.ScheduledSplit{
			{GUID: "ss1", AccountGUID: "a1", Value: domain.MustFromNumDenom(500000, 100)},
			{GUID: "ss2", AccountGUID: "a2", Value: domain.MustFromNumDenom(-500000, 100)},
		},
	}

	rec := schedReq(newSchedHandler(sr), "POST",
		"/api/v1/books/book-1/scheduled-transactions/post-due?asOf=2024-01-31T00:00:00Z", "")
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
	if len(sr.inserted) != 1 {
		t.Errorf("expected 1 transaction inserted, got %d", len(sr.inserted))
	}
}
