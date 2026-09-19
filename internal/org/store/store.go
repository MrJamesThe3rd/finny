package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/MrJamesThe3rd/finny/internal/audit"
	"github.com/MrJamesThe3rd/finny/internal/auth"
	"github.com/MrJamesThe3rd/finny/internal/database"
	"github.com/MrJamesThe3rd/finny/internal/org"
)

var _ org.Repository = (*Store)(nil)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) ListForUser(ctx context.Context, userID uuid.UUID) ([]*org.Organization, error) {
	const query = `
		SELECT o.id, o.name, COALESCE(o.nif, ''), o.created_at
		FROM organizations o
		JOIN memberships m ON m.org_id = o.id
		WHERE m.user_id = $1 AND m.revoked_at IS NULL
		ORDER BY o.created_at ASC
	`

	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("listing organizations: %w", err)
	}
	defer rows.Close()

	var orgs []*org.Organization

	for rows.Next() {
		var o org.Organization

		if err := rows.Scan(&o.ID, &o.Name, &o.NIF, &o.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning organization: %w", err)
		}

		orgs = append(orgs, &o)
	}

	return orgs, rows.Err()
}

func (s *Store) GetMembership(ctx context.Context, userID, orgID uuid.UUID) (*org.Membership, error) {
	const query = `
		SELECT id, user_id, org_id, role, created_at, revoked_at
		FROM memberships
		WHERE user_id = $1 AND org_id = $2 AND revoked_at IS NULL
	`

	var (
		m       org.Membership
		roleStr string
	)

	err := s.db.QueryRowContext(ctx, query, userID, orgID).
		Scan(&m.ID, &m.UserID, &m.OrgID, &roleStr, &m.CreatedAt, &m.RevokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		// Absent and not-yours are the same answer, deliberately — see errors.go.
		return nil, org.ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("getting membership: %w", err)
	}

	m.Role = org.Role(roleStr)

	return &m, nil
}

func (s *Store) CreateWithOwner(ctx context.Context, o *org.Organization, ownerID uuid.UUID) error {
	return database.InTx(ctx, s.db, func(tx *sql.Tx) error {
		const orgQuery = `
			INSERT INTO organizations (name, nif)
			VALUES ($1, NULLIF($2, ''))
			RETURNING id, created_at
		`

		if err := tx.QueryRowContext(ctx, orgQuery, o.Name, o.NIF).Scan(&o.ID, &o.CreatedAt); err != nil {
			return fmt.Errorf("creating organization: %w", err)
		}

		const memberQuery = `INSERT INTO memberships (user_id, org_id, role) VALUES ($1, $2, $3)`

		if _, err := tx.ExecContext(ctx, memberQuery, ownerID, o.ID, string(org.RoleOwner)); err != nil {
			return fmt.Errorf("creating owner membership: %w", err)
		}

		if err := audit.Record(ctx, tx, audit.Entry{
			OrgID:       o.ID,
			ActorUserID: ownerID,
			SubjectType: audit.SubjectOrganization,
			SubjectID:   o.ID,
			Action:      audit.ActionCreate,
			NewValue:    map[string]any{"name": o.Name, "nif": o.NIF},
		}); err != nil {
			return err
		}

		return audit.Record(ctx, tx, audit.Entry{
			OrgID:       o.ID,
			ActorUserID: ownerID,
			SubjectType: audit.SubjectMembership,
			SubjectID:   ownerID,
			Action:      audit.ActionGrant,
			NewValue:    map[string]any{"role": org.RoleOwner},
		})
	})
}

func (s *Store) AddMember(ctx context.Context, m *org.Membership) error {
	actorID := auth.UserID(ctx)

	return database.InTx(ctx, s.db, func(tx *sql.Tx) error {
		// Re-granting a revoked membership reactivates the existing row: the
		// table is append-only per (user, org), so a second row cannot exist.
		const query = `
			INSERT INTO memberships (user_id, org_id, role)
			VALUES ($1, $2, $3)
			ON CONFLICT (user_id, org_id) DO UPDATE
			    SET role = EXCLUDED.role, revoked_at = NULL
			RETURNING id, created_at
		`

		if err := tx.QueryRowContext(ctx, query, m.UserID, m.OrgID, string(m.Role)).
			Scan(&m.ID, &m.CreatedAt); err != nil {
			return fmt.Errorf("adding member: %w", err)
		}

		return audit.Record(ctx, tx, audit.Entry{
			OrgID:       m.OrgID,
			ActorUserID: actorID,
			SubjectType: audit.SubjectMembership,
			SubjectID:   m.UserID,
			Action:      audit.ActionGrant,
			NewValue:    map[string]any{"role": m.Role},
		})
	})
}

func (s *Store) RevokeMember(ctx context.Context, orgID, userID uuid.UUID) error {
	actorID := auth.UserID(ctx)

	return database.InTx(ctx, s.db, func(tx *sql.Tx) error {
		role, err := lockActiveMembers(ctx, tx, orgID, userID)
		if err != nil {
			return err
		}

		const query = `
			UPDATE memberships SET revoked_at = NOW()
			WHERE org_id = $1 AND user_id = $2 AND revoked_at IS NULL
		`

		if _, err := tx.ExecContext(ctx, query, orgID, userID); err != nil {
			return fmt.Errorf("revoking membership: %w", err)
		}

		return audit.Record(ctx, tx, audit.Entry{
			OrgID:       orgID,
			ActorUserID: actorID,
			SubjectType: audit.SubjectMembership,
			SubjectID:   userID,
			Action:      audit.ActionRevoke,
			OldValue:    map[string]any{"role": role},
		})
	})
}

// lockActiveMembers locks every active membership of the organization and
// returns the target's role. The lock is what makes the last-owner check safe:
// as a bare check-then-act, two concurrent revokes of an org's two owners both
// see a count of 2 and leave it ownerless.
func lockActiveMembers(ctx context.Context, tx *sql.Tx, orgID, userID uuid.UUID) (org.Role, error) {
	const query = `
		SELECT user_id, role FROM memberships
		WHERE org_id = $1 AND revoked_at IS NULL
		ORDER BY created_at ASC
		FOR UPDATE
	`

	rows, err := tx.QueryContext(ctx, query, orgID)
	if err != nil {
		return "", fmt.Errorf("locking memberships: %w", err)
	}
	defer rows.Close()

	var (
		targetRole org.Role
		found      bool
		owners     int
	)

	for rows.Next() {
		var (
			memberID uuid.UUID
			roleStr  string
		)

		if err := rows.Scan(&memberID, &roleStr); err != nil {
			return "", fmt.Errorf("scanning membership: %w", err)
		}

		if org.Role(roleStr) == org.RoleOwner {
			owners++
		}

		if memberID == userID {
			targetRole = org.Role(roleStr)
			found = true
		}
	}

	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterating memberships: %w", err)
	}

	if !found {
		return "", org.ErrNotFound
	}

	if targetRole == org.RoleOwner && owners <= 1 {
		return "", org.ErrLastOwner
	}

	return targetRole, nil
}
