package transaction_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestService_ImportBatch_NoConflicts(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := transaction.NewMockRepository(ctrl)
	itx := transaction.NewMockImportTx(ctrl)
	svc := transaction.NewService(repo)

	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	params := []transaction.CreateParams{
		{
			Amount:         1000,
			Type:           transaction.TypeExpense,
			Status:         transaction.StatusDraft,
			Description:    "Coffee",
			RawDescription: "COFFEE SHOP",
			Date:           date,
		},
	}

	repo.EXPECT().BeginImport(gomock.Any(), date, date).Return(itx, nil)
	itx.EXPECT().FindDuplicates(gomock.Any(), params).Return(nil, nil)
	itx.EXPECT().CreateTransactions(gomock.Any(), gomock.Any()).Return(nil)
	itx.EXPECT().Commit().Return(nil)
	itx.EXPECT().Rollback().Return(nil)

	result, err := svc.ImportBatch(context.Background(), params)
	require.NoError(t, err)
	assert.Len(t, result.Imported, 1)
	assert.Empty(t, result.Conflicts)
	assert.Empty(t, result.New)
}

func TestService_ImportBatch_WithConflicts(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := transaction.NewMockRepository(ctrl)
	itx := transaction.NewMockImportTx(ctrl)
	svc := transaction.NewService(repo)

	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	params := []transaction.CreateParams{
		{
			Amount:         1000,
			Type:           transaction.TypeExpense,
			Status:         transaction.StatusDraft,
			Description:    "Coffee",
			RawDescription: "COFFEE SHOP",
			Date:           date,
		},
		{
			Amount:         2000,
			Type:           transaction.TypeExpense,
			Status:         transaction.StatusDraft,
			Description:    "Lunch",
			RawDescription: "LUNCH PLACE",
			Date:           date,
		},
	}

	existing := &transaction.Transaction{
		ID:             uuid.New(),
		Amount:         1000,
		Type:           transaction.TypeExpense,
		RawDescription: "COFFEE SHOP",
		Date:           date,
	}

	repo.EXPECT().BeginImport(gomock.Any(), date, date).Return(itx, nil)
	itx.EXPECT().FindDuplicates(gomock.Any(), params).Return([]*transaction.Transaction{existing}, nil)
	itx.EXPECT().Rollback().Return(nil)

	result, err := svc.ImportBatch(context.Background(), params)
	require.NoError(t, err)
	assert.Empty(t, result.Imported)
	assert.Len(t, result.New, 1)
	assert.Len(t, result.Conflicts, 1)
	assert.Equal(t, params[0], result.Conflicts[0].Incoming)
	assert.Equal(t, existing, result.Conflicts[0].Existing)
}

func TestService_ImportBatch_Empty(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := transaction.NewMockRepository(ctrl)
	svc := transaction.NewService(repo)

	result, err := svc.ImportBatch(context.Background(), []transaction.CreateParams{})
	require.NoError(t, err)
	assert.Empty(t, result.Imported)
	assert.Empty(t, result.Conflicts)
	assert.Empty(t, result.New)
}

func TestService_CreateBatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := transaction.NewMockRepository(ctrl)
	itx := transaction.NewMockImportTx(ctrl)
	svc := transaction.NewService(repo)

	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	params := []transaction.CreateParams{
		{
			Amount:         1000,
			Type:           transaction.TypeExpense,
			Status:         transaction.StatusDraft,
			Description:    "Coffee",
			RawDescription: "COFFEE SHOP",
			Date:           date,
		},
	}

	repo.EXPECT().BeginImport(gomock.Any(), date, date).Return(itx, nil)
	itx.EXPECT().CreateTransactions(gomock.Any(), gomock.Any()).Return(nil)
	itx.EXPECT().Commit().Return(nil)
	itx.EXPECT().Rollback().Return(nil)

	txs, err := svc.CreateBatch(context.Background(), params)
	require.NoError(t, err)
	assert.Len(t, txs, 1)
	assert.Equal(t, int64(1000), txs[0].Amount)
	assert.Equal(t, transaction.TypeExpense, txs[0].Type)
}
