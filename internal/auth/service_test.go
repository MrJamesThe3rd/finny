package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/crypto/bcrypt"

	"github.com/MrJamesThe3rd/finny/internal/auth"
)

const svcSecret = "service-test-secret-that-is-32bytes"

func makeHash(t *testing.T, password string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	require.NoError(t, err)
	return string(h)
}

func newService(repo auth.Repository) *auth.Service {
	return auth.NewService(repo, svcSecret, 15*time.Minute, 7*24*time.Hour)
}

// --- Login ---

func TestService_Login_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := auth.NewMockRepository(ctrl)

	user := &auth.User{
		ID:           uuid.New(),
		Email:        "alice@example.com",
		PasswordHash: makeHash(t, "correctpassword"),
	}

	repo.EXPECT().GetUserByLogin(gomock.Any(), "alice@example.com").Return(user, nil)
	repo.EXPECT().CreateRefreshToken(gomock.Any(), gomock.Any()).Return(nil)
	repo.EXPECT().UpdateLastLogin(gomock.Any(), user.ID).Return(nil)

	svc := newService(repo)
	result, err := svc.Login(context.Background(), "alice@example.com", "correctpassword")

	require.NoError(t, err)
	assert.NotEmpty(t, result.AccessToken)
	assert.NotEmpty(t, result.RefreshToken)
	assert.True(t, result.ExpiresAt.After(time.Now()))
}

func TestService_Login_UserNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := auth.NewMockRepository(ctrl)

	repo.EXPECT().GetUserByLogin(gomock.Any(), "nobody@example.com").Return(nil, auth.ErrUserNotFound)

	svc := newService(repo)
	_, err := svc.Login(context.Background(), "nobody@example.com", "anypassword")

	assert.ErrorIs(t, err, auth.ErrInvalidCredentials)
}

func TestService_Login_WrongPassword(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := auth.NewMockRepository(ctrl)

	user := &auth.User{
		ID:           uuid.New(),
		PasswordHash: makeHash(t, "correctpassword"),
	}

	repo.EXPECT().GetUserByLogin(gomock.Any(), "alice@example.com").Return(user, nil)

	svc := newService(repo)
	_, err := svc.Login(context.Background(), "alice@example.com", "wrongpassword")

	assert.ErrorIs(t, err, auth.ErrInvalidCredentials)
}

// --- Refresh ---

func TestService_Refresh_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := auth.NewMockRepository(ctrl)

	rawToken := "some-raw-token-value"
	hash := auth.HashToken(rawToken)
	userID := uuid.New()

	rt := &auth.RefreshToken{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: hash,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	user := &auth.User{ID: userID}

	repo.EXPECT().GetRefreshToken(gomock.Any(), hash).Return(rt, nil)
	repo.EXPECT().GetUserByID(gomock.Any(), userID).Return(user, nil)
	repo.EXPECT().RevokeRefreshToken(gomock.Any(), hash).Return(nil)
	repo.EXPECT().CreateRefreshToken(gomock.Any(), gomock.Any()).Return(nil)
	repo.EXPECT().UpdateLastLogin(gomock.Any(), userID).Return(nil)

	svc := newService(repo)
	result, err := svc.Refresh(context.Background(), rawToken)

	require.NoError(t, err)
	assert.NotEmpty(t, result.AccessToken)
	assert.NotEmpty(t, result.RefreshToken)
	assert.NotEqual(t, rawToken, result.RefreshToken)
}

func TestService_Refresh_RevokedToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := auth.NewMockRepository(ctrl)

	rawToken := "revoked-token"
	hash := auth.HashToken(rawToken)
	revokedAt := time.Now().Add(-1 * time.Hour)

	rt := &auth.RefreshToken{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		TokenHash: hash,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		RevokedAt: &revokedAt,
	}

	repo.EXPECT().GetRefreshToken(gomock.Any(), hash).Return(rt, nil)

	svc := newService(repo)
	_, err := svc.Refresh(context.Background(), rawToken)

	assert.ErrorIs(t, err, auth.ErrTokenInvalid)
}

func TestService_Refresh_ExpiredToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := auth.NewMockRepository(ctrl)

	rawToken := "expired-token"
	hash := auth.HashToken(rawToken)

	rt := &auth.RefreshToken{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		TokenHash: hash,
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}

	repo.EXPECT().GetRefreshToken(gomock.Any(), hash).Return(rt, nil)

	svc := newService(repo)
	_, err := svc.Refresh(context.Background(), rawToken)

	assert.ErrorIs(t, err, auth.ErrTokenInvalid)
}

// --- Logout ---

func TestService_Logout_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := auth.NewMockRepository(ctrl)

	rawToken := "logout-token"
	hash := auth.HashToken(rawToken)

	repo.EXPECT().RevokeRefreshToken(gomock.Any(), hash).Return(nil)

	svc := newService(repo)
	err := svc.Logout(context.Background(), rawToken)

	assert.NoError(t, err)
}

func TestService_Logout_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := auth.NewMockRepository(ctrl)

	rawToken := "bad-token"
	dbErr := errors.New("connection refused")

	repo.EXPECT().RevokeRefreshToken(gomock.Any(), auth.HashToken(rawToken)).Return(dbErr)

	svc := newService(repo)
	err := svc.Logout(context.Background(), rawToken)

	assert.Error(t, err)
	assert.ErrorIs(t, err, dbErr) // infrastructure errors propagate
}

func TestService_Logout_AlreadyRevoked(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := auth.NewMockRepository(ctrl)

	rawToken := "already-revoked-token"

	repo.EXPECT().RevokeRefreshToken(gomock.Any(), auth.HashToken(rawToken)).Return(auth.ErrTokenNotFound)

	svc := newService(repo)
	err := svc.Logout(context.Background(), rawToken)

	assert.NoError(t, err) // idempotent — already revoked is treated as success
}

func TestService_Refresh_UserNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := auth.NewMockRepository(ctrl)

	rawToken := "some-raw-token"
	hash := auth.HashToken(rawToken)
	userID := uuid.New()

	rt := &auth.RefreshToken{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: hash,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}

	repo.EXPECT().GetRefreshToken(gomock.Any(), hash).Return(rt, nil)
	repo.EXPECT().GetUserByID(gomock.Any(), userID).Return(nil, auth.ErrUserNotFound)

	svc := newService(repo)
	_, err := svc.Refresh(context.Background(), rawToken)

	assert.Error(t, err)
}

// --- CreateUser ---

func TestService_CreateUser_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := auth.NewMockRepository(ctrl)

	repo.EXPECT().CreateUser(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, u *auth.User) error {
			assert.NotEqual(t, uuid.Nil, u.ID)
			assert.Equal(t, "alice@example.com", u.Email)
			assert.Equal(t, "alice", u.Username)
			assert.NotEmpty(t, u.PasswordHash)
			assert.NotEqual(t, "password123", u.PasswordHash) // must be hashed
			assert.False(t, u.IsAdmin)
			return nil
		},
	)

	svc := newService(repo)
	user, err := svc.CreateUser(context.Background(), auth.CreateUserParams{
		Email:    "alice@example.com",
		Username: "alice",
		Password: "password123",
	})

	require.NoError(t, err)
	assert.NotNil(t, user)
	assert.Equal(t, "alice@example.com", user.Email)
}

func TestService_CreateUser_PasswordTooShort(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := auth.NewMockRepository(ctrl)
	// repo.CreateUser must NOT be called

	svc := newService(repo)
	_, err := svc.CreateUser(context.Background(), auth.CreateUserParams{
		Email:    "alice@example.com",
		Username: "alice",
		Password: "short",
	})

	assert.Error(t, err)
}

// --- ListUsers ---

func TestService_ListUsers_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := auth.NewMockRepository(ctrl)

	repo.EXPECT().ListUsers(gomock.Any()).Return([]*auth.User{
		{ID: uuid.New(), Email: "alice@example.com"},
		{ID: uuid.New(), Email: "bob@example.com"},
	}, nil)

	svc := newService(repo)
	users, err := svc.ListUsers(context.Background())

	require.NoError(t, err)
	assert.Len(t, users, 2)
}

// --- DeleteUser ---

func TestService_DeleteUser_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := auth.NewMockRepository(ctrl)

	id := uuid.New()
	repo.EXPECT().DeleteUser(gomock.Any(), id).Return(nil)

	svc := newService(repo)
	err := svc.DeleteUser(context.Background(), id)

	assert.NoError(t, err)
}

func TestService_DeleteUser_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := auth.NewMockRepository(ctrl)

	id := uuid.New()
	repo.EXPECT().DeleteUser(gomock.Any(), id).Return(auth.ErrUserNotFound)

	svc := newService(repo)
	err := svc.DeleteUser(context.Background(), id)

	assert.ErrorIs(t, err, auth.ErrUserNotFound)
}
