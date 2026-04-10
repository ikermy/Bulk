package repo

import (
	"context"
	"database/sql"
	"time"

	"github.com/ikermy/Bulk/internal/adapters/postgres"
	"github.com/ikermy/Bulk/internal/domain"
	"github.com/ikermy/Bulk/internal/ports"
)

type BatchRepository struct {
	inner *postgres.BatchRepository
}

func NewBatchRepository(db *sql.DB) *BatchRepository {
	return &BatchRepository{inner: postgres.NewBatchRepository(db)}
}

func (r *BatchRepository) Create(ctx context.Context, b *domain.Batch) error {
	if b == nil {
		return nil
	}
	pb := r.toPostgres(b)
	return r.inner.Create(ctx, pb)
}

func (r *BatchRepository) GetByID(ctx context.Context, id string) (*domain.Batch, error) {
	pb, err := r.inner.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return pb.ToDomain(), nil
}

//func (r *BatchRepository) helper(ctx context.Context, b *domain.Batch) (*BatchRepository, error) {
//	if b == nil {
//		return nil
//	}
//	var completedAt time.Time
//	if b.CompletedAt != nil {
//		completedAt = *b.CompletedAt
//	}
//	pb := &postgres.Batch{ID: b.ID, UserID: b.UserID, Status: string(b.Status), Revision: b.Revision, FileStorageID: b.FileStorageID, TotalRows: b.TotalRows, ValidRows: b.ValidRows, ApprovedCount: b.ApprovedCount, CompletedCount: b.CompletedCount, FailedCount: b.FailedCount, CompletedAt: completedAt, TransactionIDs: b.BillingTransactionIDs}
//	return r.inner.Update(ctx, pb), nil
//}

func (r *BatchRepository) Update(ctx context.Context, b *domain.Batch) error {
	if b == nil {
		return nil
	}
	pb := r.toPostgres(b)
	return r.inner.Update(ctx, pb)
}

// toPostgres converts domain.Batch to postgres.Batch used by the adapter layer.
func (r *BatchRepository) toPostgres(b *domain.Batch) *postgres.Batch {
	if b == nil {
		return nil
	}
	var completedAt time.Time
	if b.CompletedAt != nil {
		completedAt = *b.CompletedAt
	}
	return &postgres.Batch{
		ID:            b.ID,
		UserID:        b.UserID,
		Status:        string(b.Status),
		Revision:      b.Revision,
		FileStorageID: b.FileStorageID,
		TotalRows:     b.TotalRows,
		ValidRows:     b.ValidRows,
		ApprovedCount: b.ApprovedCount,
		CompletedCount: b.CompletedCount,
		FailedCount:   b.FailedCount,
		CompletedAt:   completedAt,
		TransactionIDs: b.BillingTransactionIDs,
	}
}

func (r *BatchRepository) List(ctx context.Context, filter ports.BatchFilter) ([]*domain.Batch, int, error) {
	// calculate offset
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	offset := (filter.Page - 1) * filter.Limit
	// support cursor-based pagination if Cursor provided (RFC3339 timestamp)
	var cursorTime *time.Time
	if filter.Cursor != "" {
		if t, err := time.Parse(time.RFC3339, filter.Cursor); err == nil {
			cursorTime = &t
		}
	}
	batches, total, err := r.inner.ListWithFilters(ctx, filter.Status, filter.UserID, filter.Revision, filter.From, filter.To, filter.SortBy, filter.SortDesc, cursorTime, filter.Limit, offset)
	if err != nil {
		return nil, 0, err
	}
	var res []*domain.Batch
	for _, b := range batches {
		res = append(res, b.ToDomain())
	}
	return res, total, nil
}

// AdminStats delegates to inner postgres repository which performs aggregations
func (r *BatchRepository) AdminStats(ctx context.Context, from *time.Time, to *time.Time) (*ports.AdminStatsWithQueues, error) {
	if r.inner == nil {
		return &ports.AdminStatsWithQueues{}, nil
	}
	return r.inner.AdminStats(ctx, from, to)
}

