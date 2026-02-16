package transaction

import (
	"context"
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
