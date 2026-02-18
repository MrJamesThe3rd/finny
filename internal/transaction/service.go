package transaction

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

//go:generate mockgen -source=service.go -destination=repository_mock.go -package=transaction
type Repository interface {
	CreateTransaction(ctx context.Context, tx *Transaction) error
	GetTransaction(ctx context.Context, id uuid.UUID) (*Transaction, error)
	UpdateTransaction(ctx context.Context, tx *Transaction) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status Status) error

	ListTransactions(ctx context.Context, filter ListFilter) ([]*Transaction, error)
	DeleteTransaction(ctx context.Context, id uuid.UUID) error
	UpdateInvoice(ctx context.Context, id uuid.UUID, invoiceURL string) error

	BeginImport(ctx context.Context, minDate, maxDate time.Time) (ImportTx, error)
}

type ImportTx interface {
	FindDuplicates(ctx context.Context, params []CreateParams) ([]*Transaction, error)
	CreateTransactions(ctx context.Context, txs []*Transaction) error
	Commit() error
	Rollback() error
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

type CreateParams struct {
	Amount         int64
	Type           Type
	Status         Status
	Description    string
	RawDescription string
	Date           time.Time
}

type ListFilter struct {
	Status    *Status
	StartDate *time.Time
	EndDate   *time.Time
}

func (s *Service) Create(ctx context.Context, params CreateParams) (*Transaction, error) {
	tx := &Transaction{
		Amount:         params.Amount,
		Type:           params.Type,
		Status:         params.Status,
		Description:    params.Description,
		RawDescription: params.RawDescription,
		Date:           params.Date,
	}
	if err := s.repo.CreateTransaction(ctx, tx); err != nil {
		return nil, err
	}

	return tx, nil
}

func (s *Service) AttachInvoice(ctx context.Context, id uuid.UUID, invoiceURL string) error {
	return s.repo.UpdateInvoice(ctx, id, invoiceURL)
}

func (s *Service) List(ctx context.Context, filter ListFilter) ([]*Transaction, error) {
	return s.repo.ListTransactions(ctx, filter)
}

func (s *Service) Update(ctx context.Context, tx *Transaction) error {
	return s.repo.UpdateTransaction(ctx, tx)
}

func (s *Service) UpdateStatus(ctx context.Context, id uuid.UUID, status Status) error {
	return s.repo.UpdateStatus(ctx, id, status)
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Transaction, error) {
	return s.repo.GetTransaction(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteTransaction(ctx, id)
}

func (s *Service) UpdateInvoice(ctx context.Context, id uuid.UUID, invoiceURL string) error {
	return s.repo.UpdateInvoice(ctx, id, invoiceURL)
}

type ImportResult struct {
	Imported  []*Transaction
	New       []CreateParams
	Conflicts []Conflict
}

type Conflict struct {
	Incoming CreateParams
	Existing *Transaction
}

func (s *Service) ImportBatch(ctx context.Context, params []CreateParams) (*ImportResult, error) {
	if len(params) == 0 {
		return &ImportResult{}, nil
	}

	minDate, maxDate := dateRange(params)

	itx, err := s.repo.BeginImport(ctx, minDate, maxDate)
	if err != nil {
		return nil, fmt.Errorf("begin import: %w", err)
	}
	defer itx.Rollback()

	duplicates, err := itx.FindDuplicates(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("find duplicates: %w", err)
	}

	type dupKey struct {
		Date           string
		Amount         int64
		Type           Type
		RawDescription string
	}

	lookup := make(map[dupKey]*Transaction, len(duplicates))

	for _, d := range duplicates {
		k := dupKey{
			Date:           d.Date.Format(time.DateOnly),
			Amount:         d.Amount,
			Type:           d.Type,
			RawDescription: d.RawDescription,
		}
		lookup[k] = d
	}

	var newParams []CreateParams

	var conflicts []Conflict

	for _, p := range params {
		k := dupKey{
			Date:           p.Date.Format(time.DateOnly),
			Amount:         p.Amount,
			Type:           p.Type,
			RawDescription: p.RawDescription,
		}

		existing, found := lookup[k]
		if found {
			conflicts = append(conflicts, Conflict{Incoming: p, Existing: existing})
			continue
		}

		newParams = append(newParams, p)
	}

	if len(conflicts) > 0 {
		return &ImportResult{New: newParams, Conflicts: conflicts}, nil
	}

	txs := paramsToTransactions(newParams)
	if err := itx.CreateTransactions(ctx, txs); err != nil {
		return nil, fmt.Errorf("create transactions: %w", err)
	}

	if err := itx.Commit(); err != nil {
		return nil, fmt.Errorf("commit import: %w", err)
	}

	return &ImportResult{Imported: txs}, nil
}

func (s *Service) CreateBatch(ctx context.Context, params []CreateParams) ([]*Transaction, error) {
	if len(params) == 0 {
		return nil, nil
	}

	minDate, maxDate := dateRange(params)

	itx, err := s.repo.BeginImport(ctx, minDate, maxDate)
	if err != nil {
		return nil, fmt.Errorf("begin import: %w", err)
	}
	defer itx.Rollback()

	txs := paramsToTransactions(params)
	if err := itx.CreateTransactions(ctx, txs); err != nil {
		return nil, fmt.Errorf("create transactions: %w", err)
	}

	if err := itx.Commit(); err != nil {
		return nil, fmt.Errorf("commit import: %w", err)
	}

	return txs, nil
}

func dateRange(params []CreateParams) (time.Time, time.Time) {
	minDate := params[0].Date
	maxDate := params[0].Date

	for _, p := range params[1:] {
		if p.Date.Before(minDate) {
			minDate = p.Date
		}

		if p.Date.After(maxDate) {
			maxDate = p.Date
		}
	}

	return minDate, maxDate
}

func paramsToTransactions(params []CreateParams) []*Transaction {
	txs := make([]*Transaction, len(params))
	for i, p := range params {
		txs[i] = &Transaction{
			Amount:         p.Amount,
			Type:           p.Type,
			Status:         p.Status,
			Description:    p.Description,
			RawDescription: p.RawDescription,
			Date:           p.Date,
		}
	}

	return txs
}
