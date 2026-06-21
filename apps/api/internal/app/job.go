package app

import (
	"context"

	"github.com/openledger/openledger/apps/api/internal/domain"
)

// JobRepository is the persistence port for jobs.
type JobRepository interface {
	ListJobs(ctx context.Context, bookGUID string, activeOnly bool) ([]domain.Job, error)
	CreateJob(ctx context.Context, j domain.Job) error
	GetJob(ctx context.Context, guid string) (domain.Job, error)
	UpdateJob(ctx context.Context, j domain.Job) error
	DeleteJob(ctx context.Context, guid string) error
}

// JobService manages jobs within a book.
type JobService struct {
	repo    JobRepository
	authz   *AuthzService
	newGUID func() string
}

// NewJobService creates a JobService.
func NewJobService(repo JobRepository, authz *AuthzService) *JobService {
	return &JobService{repo: repo, authz: authz, newGUID: NewGUID}
}

// ListJobs returns all jobs for a book.
func (s *JobService) ListJobs(ctx context.Context, bookGUID, userID string, activeOnly bool) ([]domain.Job, error) {
	if err := s.authz.AuthorizeBook(ctx, userID, bookGUID, AccessRead); err != nil {
		return nil, err
	}
	return s.repo.ListJobs(ctx, bookGUID, activeOnly)
}

// CreateJob adds a new job to a book.
func (s *JobService) CreateJob(ctx context.Context, userID string, j domain.Job) (domain.Job, error) {
	if err := s.authz.AuthorizeBook(ctx, userID, j.BookGUID, AccessWrite); err != nil {
		return domain.Job{}, err
	}
	if j.Name == "" || !validOwnerType(j.OwnerType) || j.OwnerGUID == "" {
		return domain.Job{}, ErrInvalidInput
	}
	j.GUID = s.newGUID()
	j.Active = true
	if err := s.repo.CreateJob(ctx, j); err != nil {
		return domain.Job{}, err
	}
	return s.repo.GetJob(ctx, j.GUID)
}

// GetJob returns a single job by GUID.
func (s *JobService) GetJob(ctx context.Context, guid, userID string) (domain.Job, error) {
	j, err := s.repo.GetJob(ctx, guid)
	if err != nil {
		return domain.Job{}, err
	}
	if err := s.authz.AuthorizeBook(ctx, userID, j.BookGUID, AccessRead); err != nil {
		return domain.Job{}, err
	}
	return j, nil
}

// UpdateJob patches a job's mutable fields. The owner is fixed at creation.
func (s *JobService) UpdateJob(ctx context.Context, userID string, j domain.Job) (domain.Job, error) {
	existing, err := s.repo.GetJob(ctx, j.GUID)
	if err != nil {
		return domain.Job{}, err
	}
	if err := s.authz.AuthorizeBook(ctx, userID, existing.BookGUID, AccessWrite); err != nil {
		return domain.Job{}, err
	}
	if j.Name == "" {
		return domain.Job{}, ErrInvalidInput
	}
	j.BookGUID = existing.BookGUID
	j.OwnerType = existing.OwnerType
	j.OwnerGUID = existing.OwnerGUID
	j.CreatedAt = existing.CreatedAt
	if err := s.repo.UpdateJob(ctx, j); err != nil {
		return domain.Job{}, err
	}
	return s.repo.GetJob(ctx, j.GUID)
}

// DeleteJob removes a job. Returns ErrJobNotFound if it does not exist.
func (s *JobService) DeleteJob(ctx context.Context, guid, userID string) error {
	existing, err := s.repo.GetJob(ctx, guid)
	if err != nil {
		return err
	}
	if err := s.authz.AuthorizeBook(ctx, userID, existing.BookGUID, AccessWrite); err != nil {
		return err
	}
	return s.repo.DeleteJob(ctx, guid)
}

func validOwnerType(t string) bool {
	return t == "customer" || t == "vendor"
}
