package transaction_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

func TestService_Create(t *testing.T) {
	type args struct {
		params transaction.CreateParams
	}

	type testCase struct {
		name      string
		args      args
		setupMock func(m *transaction.MockRepository)
		wantErr   bool
	}

	tests := []testCase{
		{
			name: "Success",
			args: args{
				params: transaction.CreateParams{
					Amount:      1000,
					Type:        transaction.TypeExpense,
					Status:      transaction.StatusComplete,
					Description: "Test Transaction",
					Date:        time.Date(2023, 10, 27, 0, 0, 0, 0, time.UTC),
				},
			},
			setupMock: func(m *transaction.MockRepository) {
				m.EXPECT().
					CreateTransaction(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, tx *transaction.Transaction) error {
						tx.ID = uuid.New()
						tx.CreatedAt = time.Now()
						return nil
					})
			},
			wantErr: false,
		},
		{
			name: "RepoError",
			args: args{
				params: transaction.CreateParams{
					Amount: 500,
				},
			},
			setupMock: func(m *transaction.MockRepository) {
				m.EXPECT().
					CreateTransaction(gomock.Any(), gomock.Any()).
					Return(errors.New("db error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			repo := transaction.NewMockRepository(ctrl)
			if tt.setupMock != nil {
				tt.setupMock(repo)
			}

			svc := transaction.NewService(repo)
			got, err := svc.Create(context.Background(), tt.args.params)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, got)

				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, got)
			assert.NotEmpty(t, got.ID)
		})
	}
}

func TestService_List(t *testing.T) {
	type args struct {
		filter transaction.ListFilter
	}

	type testCase struct {
		name      string
		args      args
		setupMock func(m *transaction.MockRepository)
		wantLen   int
		wantErr   bool
	}

	tests := []testCase{
		{
			name: "Success",
			args: args{filter: transaction.ListFilter{}},
			setupMock: func(m *transaction.MockRepository) {
				m.EXPECT().
					ListTransactions(gomock.Any(), transaction.ListFilter{}).
					Return([]*transaction.Transaction{
						{ID: uuid.New()},
						{ID: uuid.New()},
					}, nil)
			},
			wantLen: 2,
			wantErr: false,
		},
		{
			name: "Error",
			args: args{filter: transaction.ListFilter{}},
			setupMock: func(m *transaction.MockRepository) {
				m.EXPECT().
					ListTransactions(gomock.Any(), transaction.ListFilter{}).
					Return(nil, errors.New("list error"))
			},
			wantLen: 0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			repo := transaction.NewMockRepository(ctrl)
			if tt.setupMock != nil {
				tt.setupMock(repo)
			}

			svc := transaction.NewService(repo)
			got, err := svc.List(context.Background(), tt.args.filter)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Len(t, got, tt.wantLen)
		})
	}
}
