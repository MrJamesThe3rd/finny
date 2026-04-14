package auth

import "errors"

var (
	// ErrUserNotFound is returned when a user lookup finds no matching row.
	ErrUserNotFound = errors.New("user not found")

	// ErrInvalidCredentials is returned for bad login attempts. Always use this
	// instead of more specific errors to avoid leaking which field was wrong.
	ErrInvalidCredentials = errors.New("invalid credentials")

	// ErrTokenInvalid is returned when a refresh token is missing, expired, or revoked.
	ErrTokenInvalid = errors.New("invalid or expired token")

	// ErrTokenNotFound is returned by the repository when a refresh token hash
	// does not exist in the store. The service layer maps this to ErrTokenInvalid.
	ErrTokenNotFound = errors.New("refresh token not found")

	// ErrPasswordTooShort is returned when a password does not meet the minimum length.
	ErrPasswordTooShort = errors.New("password must be at least 8 characters")
)
