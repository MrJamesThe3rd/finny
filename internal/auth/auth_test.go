package auth_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/MrJamesThe3rd/finny/internal/auth"
)

const testSecret = "this-is-a-test-secret-long-enough-32b"

func TestSignAndVerifyToken(t *testing.T) {
	user := &auth.User{ID: uuid.New(), IsAdmin: true}

	tokenStr, err := auth.SignAccessToken(user, testSecret, 15*time.Minute)
	require.NoError(t, err)
	assert.NotEmpty(t, tokenStr)

	claims, err := auth.VerifyToken(tokenStr, testSecret)
	require.NoError(t, err)
	assert.Equal(t, user.ID, claims.UserID)
	assert.True(t, claims.IsAdmin)
	assert.NotEmpty(t, claims.ID) // jti populated
}

func TestVerifyToken_Expired(t *testing.T) {
	user := &auth.User{ID: uuid.New()}

	tokenStr, err := auth.SignAccessToken(user, testSecret, -1*time.Second)
	require.NoError(t, err)

	_, err = auth.VerifyToken(tokenStr, testSecret)
	assert.Error(t, err)
}

func TestVerifyToken_WrongSecret(t *testing.T) {
	user := &auth.User{ID: uuid.New()}

	tokenStr, err := auth.SignAccessToken(user, testSecret, 15*time.Minute)
	require.NoError(t, err)

	_, err = auth.VerifyToken(tokenStr, "completely-different-secret-here-32b")
	assert.Error(t, err)
}

func TestVerifyToken_Tampered(t *testing.T) {
	user := &auth.User{ID: uuid.New()}

	tokenStr, err := auth.SignAccessToken(user, testSecret, 15*time.Minute)
	require.NoError(t, err)

	_, err = auth.VerifyToken(tokenStr+"x", testSecret)
	assert.Error(t, err)
}

func TestHashToken_Deterministic(t *testing.T) {
	h1 := auth.HashToken("mytoken")
	h2 := auth.HashToken("mytoken")
	h3 := auth.HashToken("differenttoken")

	assert.Equal(t, h1, h2)
	assert.NotEqual(t, h1, h3)
	assert.NotEmpty(t, h1)
	assert.Len(t, h1, 64)
	assert.Regexp(t, `^[0-9a-f]+$`, h1)
}

func TestGenerateRefreshToken_Unique(t *testing.T) {
	t1, err := auth.GenerateRefreshToken()
	require.NoError(t, err)

	t2, err := auth.GenerateRefreshToken()
	require.NoError(t, err)

	assert.NotEqual(t, t1, t2)
	assert.NotEmpty(t, t1)
	assert.Len(t, t1, 43) // 32 bytes, base64 RawURL = 43 chars
}
