package app

import "context"

// BookPreferences holds the editable per-book settings.
type BookPreferences struct {
	// DefaultCommodityGUID is the GUID of the commodity used as the book's home
	// currency. Empty means "not set". Nullable in the DB; see migration 0013.
	DefaultCommodityGUID string
	// FiscalYearStart is the month (1–12) on which the fiscal year begins.
	// 1 = January (calendar year). The fiscal year end is always the last day of
	// month FiscalYearStart−1 (wrapping to December). Stored in migration 0016.
	FiscalYearStart int
}

// PreferencesRepository is the persistence port for book preferences.
type PreferencesRepository interface {
	// GetBookPreferences returns the stored preferences for a book. If no row
	// exists yet it returns an empty BookPreferences (zero value), not an error.
	GetBookPreferences(ctx context.Context, bookGUID string) (BookPreferences, error)
	// UpsertBookPreferences writes (or overwrites) the preferences for a book.
	UpsertBookPreferences(ctx context.Context, bookGUID string, prefs BookPreferences) error
}

// PreferencesService manages the editable settings for a book.
type PreferencesService struct {
	repo  PreferencesRepository
	authz *AuthzService
}

// NewPreferencesService constructs a PreferencesService.
func NewPreferencesService(repo PreferencesRepository, authz *AuthzService) *PreferencesService {
	return &PreferencesService{repo: repo, authz: authz}
}

// GetPreferences returns the preferences for a book. Any member with read
// access may fetch them.
func (s *PreferencesService) GetPreferences(ctx context.Context, userID, bookGUID string) (BookPreferences, error) {
	if err := s.authz.AuthorizeBook(ctx, userID, bookGUID, AccessRead); err != nil {
		return BookPreferences{}, err
	}
	return s.repo.GetBookPreferences(ctx, bookGUID)
}

// UpdatePreferences saves the preferences for a book. Admin access is required
// so viewers and editors cannot change book-wide settings.
func (s *PreferencesService) UpdatePreferences(ctx context.Context, userID, bookGUID string, prefs BookPreferences) error {
	if err := s.authz.AuthorizeBook(ctx, userID, bookGUID, AccessAdmin); err != nil {
		return err
	}
	return s.repo.UpsertBookPreferences(ctx, bookGUID, prefs)
}
