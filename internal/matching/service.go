package matching

import (
	"context"
)

type Repository interface {
	FindMatch(ctx context.Context, rawDescription string) (string, error)
	CreateMapping(ctx context.Context, rawPattern, preferredDescription string) error
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Suggest tries to find a preferred description for the given raw description.
// Returns empty string if no match found.
func (s *Service) Suggest(ctx context.Context, rawDescription string) (string, error) {
	return s.repo.FindMatch(ctx, rawDescription)
}

// Learn remembers a new mapping between a raw pattern and a preferred description.
func (s *Service) Learn(ctx context.Context, rawPattern, preferredDescription string) error {
	return s.repo.CreateMapping(ctx, rawPattern, preferredDescription)
}
