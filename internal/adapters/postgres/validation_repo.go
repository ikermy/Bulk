package postgres

import (
    "context"
    "database/sql"

    "github.com/ikermy/Bulk/internal/ports"
)

type ValidationRepository struct {
    db *sql.DB
}

func NewValidationRepository(db *sql.DB) *ValidationRepository { return &ValidationRepository{db: db} }

func (r *ValidationRepository) GetValidationErrors(ctx context.Context, batchID string) ([]*ports.ValidationError, error) {
    rows, err := r.db.QueryContext(ctx, "SELECT row_number, field, error_code, error_message, original_value FROM batch_validation_errors WHERE batch_id=$1 ORDER BY row_number", batchID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var res []*ports.ValidationError
    for rows.Next() {
        var ve ports.ValidationError
        if err := rows.Scan(&ve.RowNumber, &ve.Field, &ve.ErrorCode, &ve.ErrorMessage, &ve.OriginalValue); err != nil {
            return nil, err
        }
        res = append(res, &ve)
    }
    return res, nil
}

