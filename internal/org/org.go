// Package org holds the tenant boundary. An organization is a set of books —
// a client company, or a personal account — and a membership is the grant that
// lets a user see them.
package org

import (
	"time"

	"github.com/google/uuid"
)

// Role is a user's role within one organization. Mirrors the CHECK constraint
// on memberships.role.
type Role string

const (
	RoleOwner      Role = "owner"
	RoleAccountant Role = "accountant"
)

// Valid reports whether r is a role the memberships CHECK constraint accepts.
func (r Role) Valid() bool {
	return r == RoleOwner || r == RoleAccountant
}

// Organization is one set of books. NIF is optional — a personal org has none
// and a non-PT client has a different identifier. Empty means absent.
type Organization struct {
	ID        uuid.UUID
	Name      string
	NIF       string
	CreatedAt time.Time
}

// Membership grants one user access to one organization. Rows are append-only:
// revocation sets RevokedAt so the record that someone had access during a
// period they signed off survives.
type Membership struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	OrgID     uuid.UUID
	Role      Role
	CreatedAt time.Time
	RevokedAt *time.Time
}

// ValidateNIF checks the mod-11 check digit of a Portuguese NIF.
// An empty nif is valid: the field is optional by design.
func ValidateNIF(nif string) error {
	if nif == "" {
		return nil
	}

	if len(nif) != 9 {
		return ErrInvalidNIF
	}

	sum := 0

	for i := range 8 {
		d := nif[i]
		if d < '0' || d > '9' {
			return ErrInvalidNIF
		}

		sum += int(d-'0') * (9 - i)
	}

	last := nif[8]
	if last < '0' || last > '9' {
		return ErrInvalidNIF
	}

	check := 11 - sum%11
	if check >= 10 {
		check = 0
	}

	if int(last-'0') != check {
		return ErrInvalidNIF
	}

	return nil
}
