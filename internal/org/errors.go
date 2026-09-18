package org

import "errors"

var (
	// ErrNotFound covers both "no such organization" and "you are not a member
	// of it". The two must never be distinguished at the HTTP boundary:
	// telling them apart turns every scoped endpoint into an existence oracle
	// for other people's organizations. One sentinel, so no future handler can
	// map one of them to a 404.
	ErrNotFound = errors.New("organization not found")

	// ErrNoOrgContext is returned when ctx carries no membership. A store that
	// gets this must fail loudly rather than return an empty result set — an
	// unscoped query is a tenancy bug, not an empty organization.
	ErrNoOrgContext = errors.New("no organization in context")

	// ErrInvalidNIF is returned when a NIF is present but fails the mod-11 check.
	ErrInvalidNIF = errors.New("invalid NIF")

	// ErrInvalidRole is returned for a role outside the memberships CHECK constraint.
	ErrInvalidRole = errors.New("invalid role")

	// ErrLastOwner is returned when revoking would leave an organization with
	// no active owner, permanently 403ing its owner-only routes.
	ErrLastOwner = errors.New("cannot revoke the organization's last owner")
)
