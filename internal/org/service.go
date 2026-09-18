package org

import (
	"context"

	"github.com/google/uuid"
)

//go:generate mockgen -source=service.go -destination=repository_mock.go -package=org
type Repository interface {
	ListForUser(ctx context.Context, userID uuid.UUID) ([]*Organization, error)
	// GetMembership returns the user's active membership, or ErrNotFound if it
	// does not exist or has been revoked.
	GetMembership(ctx context.Context, userID, orgID uuid.UUID) (*Membership, error)
	// CreateWithOwner creates the organization and its owner membership in one
	// transaction: an organization with no owner is unreachable.
	CreateWithOwner(ctx context.Context, o *Organization, ownerID uuid.UUID) error
	AddMember(ctx context.Context, m *Membership) error
	RevokeMember(ctx context.Context, orgID, userID uuid.UUID) error
}

// Service is the organization and membership domain logic.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// ListForUser returns every organization the user currently has access to.
func (s *Service) ListForUser(ctx context.Context, userID uuid.UUID) ([]*Organization, error) {
	return s.repo.ListForUser(ctx, userID)
}

// GetMembership is the authorization lookup behind the X-Org-ID header.
func (s *Service) GetMembership(ctx context.Context, userID, orgID uuid.UUID) (*Membership, error) {
	return s.repo.GetMembership(ctx, userID, orgID)
}

// CreateOrgParams holds the inputs for creating an organization.
type CreateOrgParams struct {
	Name string
	NIF  string
}

// Create creates an organization owned by ownerID.
func (s *Service) Create(ctx context.Context, ownerID uuid.UUID, params CreateOrgParams) (*Organization, error) {
	if err := ValidateNIF(params.NIF); err != nil {
		return nil, err
	}

	o := &Organization{Name: params.Name, NIF: params.NIF}
	if err := s.repo.CreateWithOwner(ctx, o, ownerID); err != nil {
		return nil, err
	}

	return o, nil
}

// AddMember grants userID the given role in orgID. Re-granting a revoked
// membership reactivates the existing row rather than adding a second one.
func (s *Service) AddMember(ctx context.Context, orgID, userID uuid.UUID, role Role) error {
	if !role.Valid() {
		return ErrInvalidRole
	}

	return s.repo.AddMember(ctx, &Membership{OrgID: orgID, UserID: userID, Role: role})
}

// RevokeMember revokes a membership. Refuses to revoke an organization's last
// active owner — the same invariant as Create, and the likelier way to lose it.
func (s *Service) RevokeMember(ctx context.Context, orgID, userID uuid.UUID) error {
	return s.repo.RevokeMember(ctx, orgID, userID)
}
