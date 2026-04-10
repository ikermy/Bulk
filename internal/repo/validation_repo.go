package repo

import (
    "context"
    "database/sql"

    "github.com/ikermy/Bulk/internal/adapters/postgres"
    "github.com/ikermy/Bulk/internal/ports"
)

type ValidationRepository struct {
    inner *postgres.ValidationRepository
}

func NewValidationRepository(db *sql.DB) *ValidationRepository {
    return &ValidationRepository{inner: postgres.NewValidationRepository(db)}
}

func (r *ValidationRepository) GetValidationErrors(ctx context.Context, batchID string) ([]*ports.ValidationError, error) {
    return r.inner.GetValidationErrors(ctx, batchID)
}

