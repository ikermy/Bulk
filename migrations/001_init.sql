-- Схема БД ТЗ §4.2 (Database Schema)
-- Batches
CREATE TABLE IF NOT EXISTS batches (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    status VARCHAR(50) NOT NULL,
    revision VARCHAR(100) NOT NULL,
    file_storage_id UUID NOT NULL,
    total_rows INT NOT NULL,
    valid_rows INT NOT NULL,
    approved_count INT,
    completed_count INT DEFAULT 0,
    failed_count INT DEFAULT 0,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    completed_at TIMESTAMP,
    -- transaction_ids: список идентификаторов транзакций, зарезервированных в Billing
    -- Используется для аудита и отката (см. ТЗ §9.2 Pre-approved billing)
    transaction_ids JSONB
);

CREATE INDEX IF NOT EXISTS idx_batches_user_id ON batches(user_id);
CREATE INDEX IF NOT EXISTS idx_batches_status ON batches(status);

-- Batch Jobs
CREATE TABLE IF NOT EXISTS batch_jobs (
    id UUID PRIMARY KEY,
    batch_id UUID REFERENCES batches(id),
    row_number INT NOT NULL,
    status VARCHAR(50) NOT NULL,
    input_data JSONB NOT NULL,
    build_id UUID,
    barcode_urls JSONB,
    error_code VARCHAR(100),
    error_message TEXT,
    billing_transaction_id UUID,
    queued_at TIMESTAMP,
    completed_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_batch_jobs_batch_id ON batch_jobs(batch_id);
CREATE INDEX IF NOT EXISTS idx_batch_jobs_status ON batch_jobs(status);

-- Validation Errors
CREATE TABLE IF NOT EXISTS batch_validation_errors (
    id UUID PRIMARY KEY,
    batch_id UUID REFERENCES batches(id),
    row_number INT NOT NULL,
    field VARCHAR(50),
    error_code VARCHAR(100),
    error_message TEXT,
    original_value TEXT
);

