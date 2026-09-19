package org_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/MrJamesThe3rd/finny/internal/org"
)

func TestService_Create(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		give      org.CreateOrgParams
		setupMock func(m *org.MockRepository)
		wantErr   error
	}{
		{
			name: "PersonalOrgNeedsNoNIF",
			give: org.CreateOrgParams{Name: "Personal"},
			setupMock: func(m *org.MockRepository) {
				m.EXPECT().CreateWithOwner(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			},
		},
		{
			name: "ValidNIF",
			give: org.CreateOrgParams{Name: "VibrantGarden", NIF: "517948974"},
			setupMock: func(m *org.MockRepository) {
				m.EXPECT().CreateWithOwner(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			},
		},
		{
			name:    "InvalidNIFNeverReachesTheStore",
			give:    org.CreateOrgParams{Name: "Typo", NIF: "517948975"},
			wantErr: org.ErrInvalidNIF,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := org.NewMockRepository(ctrl)

			if tt.setupMock != nil {
				tt.setupMock(repo)
			}

			got, err := org.NewService(repo).Create(context.Background(), uuid.New(), tt.give)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, got)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.give.Name, got.Name)
		})
	}
}

func TestService_AddMember(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	userID := uuid.New()

	tests := []struct {
		name      string
		giveRole  org.Role
		setupMock func(m *org.MockRepository)
		wantErr   error
	}{
		{
			name:     "Accountant",
			giveRole: org.RoleAccountant,
			setupMock: func(m *org.MockRepository) {
				m.EXPECT().
					AddMember(gomock.Any(), &org.Membership{OrgID: orgID, UserID: userID, Role: org.RoleAccountant}).
					Return(nil)
			},
		},
		{
			name:     "UnknownRoleNeverReachesTheStore",
			giveRole: org.Role("superuser"),
			wantErr:  org.ErrInvalidRole,
		},
		{
			name:     "EmptyRoleNeverReachesTheStore",
			giveRole: org.Role(""),
			wantErr:  org.ErrInvalidRole,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := org.NewMockRepository(ctrl)

			if tt.setupMock != nil {
				tt.setupMock(repo)
			}

			err := org.NewService(repo).AddMember(context.Background(), orgID, userID, tt.giveRole)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestService_RevokeMember_PropagatesLastOwner(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	repo := org.NewMockRepository(ctrl)

	orgID, userID := uuid.New(), uuid.New()
	repo.EXPECT().RevokeMember(gomock.Any(), orgID, userID).Return(org.ErrLastOwner)

	err := org.NewService(repo).RevokeMember(context.Background(), orgID, userID)
	assert.ErrorIs(t, err, org.ErrLastOwner)
}

func TestService_GetMembership_PropagatesNotFound(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	repo := org.NewMockRepository(ctrl)

	userID, orgID := uuid.New(), uuid.New()
	repo.EXPECT().GetMembership(gomock.Any(), userID, orgID).Return(nil, org.ErrNotFound)

	_, err := org.NewService(repo).GetMembership(context.Background(), userID, orgID)
	assert.ErrorIs(t, err, org.ErrNotFound)
}

func TestService_ListForUser_PropagatesError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	repo := org.NewMockRepository(ctrl)

	wantErr := errors.New("db is down")
	userID := uuid.New()
	repo.EXPECT().ListForUser(gomock.Any(), userID).Return(nil, wantErr)

	_, err := org.NewService(repo).ListForUser(context.Background(), userID)
	assert.ErrorIs(t, err, wantErr)
}
