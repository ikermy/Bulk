package bulk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/ikermy/Bulk/internal/batch"
	"github.com/ikermy/Bulk/internal/billing"
	"github.com/ikermy/Bulk/internal/domain"
	"github.com/ikermy/Bulk/internal/limits"
	"github.com/ikermy/Bulk/internal/logging"
	"github.com/ikermy/Bulk/internal/metrics"
	"github.com/ikermy/Bulk/internal/parser"
	"github.com/ikermy/Bulk/internal/ports"
	"github.com/ikermy/Bulk/internal/storage"
	"github.com/ikermy/Bulk/internal/validation"
	pkgxls "github.com/ikermy/Bulk/pkg/xls"
	"github.com/xuri/excelize/v2"
	"go.opentelemetry.io/otel/trace"
)

type Service struct {
	batchRepo ports.BatchRepository
	jobRepo   ports.JobRepository
	// billing, producer etc can be added here if needed
	validator *validation.BFFValidator
	billing   ports.BillingClient
	producer  ports.KafkaProducer
	storage   *storage.FileClient
	Logger    logging.Logger
}

func NewService(batchRepo ports.BatchRepository, jobRepo ports.JobRepository, validator *validation.BFFValidator, billingClient ports.BillingClient, producer ports.KafkaProducer, storageClient *storage.FileClient) *Service {
	return &Service{batchRepo: batchRepo, jobRepo: jobRepo, validator: validator, billing: billingClient, producer: producer, storage: storageClient}
}

type CreateBatchResult struct {
	BatchID     string
	TotalRows   int
	ValidRows   int
	InvalidRows int
	Errors      []ports.ValidationError
}

// CreateBatchFromFile — парсинг загруженного XLS/CSV (pkg/xls), валидация через Validation BFF,
// сохранение в File Storage (если настроен) и создание job-ов/записи batch
func (s *Service) CreateBatchFromFile(ctx context.Context, r io.Reader, revision string) (*CreateBatchResult, error) {
	// read entire content to buffer so we can both parse and upload
	// ВАЖНО: здесь мы читаем весь файл в память. Это приемлемо при разумных лимитах
	// (контролируемых через `limits`), но для очень больших файлов это приведёт к
	// высокому потреблению памяти. При необходимости обработки больших объёмов
	// следует перейти на стриминговый парсинг (например, стриминг CSV) чтобы избежать
	// аллокации всего файла в RAM.
	var buf bytes.Buffer
	// determine runtime limits early
	l := limits.Get()
	// defensively limit reading to configured maxFileBytes + 1 to detect oversized uploads
	maxFileBytes := int64(l.MaxFileSize)
	n, err := io.CopyN(&buf, r, maxFileBytes+1)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if n > maxFileBytes {
		return nil, fmt.Errorf("file too large")
	}

	// parse and enforce limits via parser package (recognizes empty file, too many rows, invalid headers)
	// determine limits from env or defaults (l already fetched above)
	maxRows := l.MaxRowsPerBatch
	if v := os.Getenv("MAX_ROWS_PER_BATCH"); v != "" {
		if tmp, terr := strconv.Atoi(v); terr == nil {
			maxRows = tmp
		}
	}
	maxFileBytes = int64(l.MaxFileSize)
	if v := os.Getenv("MAX_FILE_SIZE_MB"); v != "" {
		if tmp, terr := strconv.Atoi(v); terr == nil {
			maxFileBytes = int64(tmp) * 1024 * 1024
		}
	}

	p := parser.NewXLSParser(maxRows, 0, maxFileBytes)
	pres, perr := p.Parse(bytes.NewReader(buf.Bytes()), revision)
	if perr != nil {
		return nil, perr
	}
	total := len(pres.Rows)

	batchID := uuid.New().String()
	b := &domain.Batch{ID: batchID, UserID: "", Status: domain.BatchStatusPending, Revision: revision, FileStorageID: "", TotalRows: total, ValidRows: total}
	// logger helper
	if s.batchRepo != nil {
		if err := s.batchRepo.Create(ctx, b); err != nil {
			return nil, err
		}
	}
	// structured log: batch created after persisting initial record
	// try to extract trace id from context
	if s.Logger != nil {
		traceID := ""
		if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
			traceID = fmt.Sprintf("%v", sc.TraceID())
		}
		s.Logger.Info("batch_created", "traceId", traceID, "batchId", batchID, "userId", b.UserID, "totalRows", total)
	}

	// publish per-batch row count metric for SLO filtering (batch size) and observe histogram
	metrics.BatchRowsTotal.WithLabelValues(batchID).Set(float64(total))
	metrics.RowsPerBatch.Observe(float64(total))
	// set last update timestamp
	metrics.BatchLastUpdateTimestamp.WithLabelValues(batchID).Set(float64(time.Now().Unix()))
	validCount := 0
	invalidCount := 0
	var validationErrors []ports.ValidationError

	// ТЗ §6.1: Local режим — предварительная проверка форматов/обязательности полей
	// до обращения к BFF. Порядок: Local → BFF → Billing (см. ТЗ §6.1 пункты 4–5).
	localVal := validation.NewLocalValidator(revision)

	// validate rows via LocalValidator (pre-check) then BFFValidator if available
	for i, row := range pres.Rows {
		fields := map[string]string{}
		// pres.Rows already contains mapped field codes per parser; use those
		for k, v := range row {
			if k == "_row" {
				continue
			}
			fields[k] = v
		}

		valid := true

		// Шаг 1: Local валидация (ТЗ §6.1 п.4) — формат и обязательность полей.
		// Не требует сетевого вызова; отклоняет синтаксически невалидные строки сразу.
		if lr, _ := localVal.ValidateRow(ctx, fields, revision); lr != nil && !lr.Valid {
			valid = false
			for _, e := range lr.Errors {
				validationErrors = append(validationErrors, ports.ValidationError{RowNumber: i, Field: "", ErrorCode: e.Code, ErrorMessage: e.Message, OriginalValue: ""})
			}
		}

		// Шаг 2: BFF валидация (ТЗ §6.1 п.3) — бизнес-правила и база данных.
		// Вызывается только если строка прошла Local-проверку.
		if valid && s.validator != nil {
			vr, err := s.validator.ValidateRow(ctx, fields, revision)
			if err != nil {
				// treat validation call error as invalid row but record a generic error
				valid = false
				validationErrors = append(validationErrors, ports.ValidationError{RowNumber: i, Field: "", ErrorCode: "VALIDATOR_ERROR", ErrorMessage: err.Error(), OriginalValue: ""})
			} else if vr == nil || !vr.Valid {
				valid = false
				// collect detailed errors if provided by validator
				if vr != nil {
					for _, e := range vr.Errors {
						// vr.Errors now содержит структурированные ValidationError (Code, Message)
						validationErrors = append(validationErrors, ports.ValidationError{RowNumber: i, Field: "", ErrorCode: e.Code, ErrorMessage: e.Message, OriginalValue: ""})
					}
				} else {
					validationErrors = append(validationErrors, ports.ValidationError{RowNumber: i, Field: "", ErrorCode: "INVALID_ROW", ErrorMessage: "row validation failed", OriginalValue: ""})
				}
			}
		}

		if valid {
			validCount++
		} else {
			invalidCount++
		}

		if s.jobRepo != nil {
			// сохраняем исходные поля строки в Job.InputData чтобы при публикации в Kafka
			// можно было положить в сообщение `fields` (см. п.7.2 ТЗ)
			// Примечание: в ТЗ (п.7.2) формат задачи для BFF должен содержать поля: userId, batchId, buildId, rowNumber, revision, fields, billingPreApproved, transactionId
			// Здесь мы сериализуем map[string]string в JSON и сохраняем в InputData для последующей публикации.
			var inputBytes []byte
			if bjson, jerr := json.Marshal(fields); jerr == nil {
				inputBytes = bjson
			} else {
				inputBytes = []byte("{}")
			}
			// rowNumber stored in parser as string under _row
			rowNum := i + 2 // default if _row missing
			if v, ok := row["_row"]; ok {
				if rn, rerr := strconv.Atoi(v); rerr == nil {
					rowNum = rn
				}
			}
			j := &domain.Job{ID: uuid.New().String(), BatchID: batchID, RowNumber: rowNum, Status: domain.JobStatusPending, InputData: string(inputBytes)}
			if err := s.jobRepo.Create(ctx, j); err == nil {
				metrics.BatchJobsTotal.WithLabelValues(batchID, string(batch.JobStatusPending)).Inc()
				if s.Logger != nil {
					traceID := ""
					if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
						traceID = fmt.Sprintf("%v", sc.TraceID())
					}
					s.Logger.Info("job_created", "traceId", traceID, "jobId", j.ID, "batchId", batchID, "rowNumber", i)
				}
			}
		}
	}

	// update batch valid_rows
	b.ValidRows = validCount
	// persist updates: valid rows and file storage id (upload file if storage configured)
	if s.storage != nil {
		// store file under batch id
		objName := batchID + ".xlsx"
		sid, err := s.storage.Save(objName, bytes.NewReader(buf.Bytes()))
		if err != nil {
			return nil, err
		}
		b.FileStorageID = sid
		if s.Logger != nil {
			traceID := ""
			if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
				traceID = fmt.Sprintf("%v", sc.TraceID())
			}
			s.Logger.Info("file_stored", "traceId", traceID, "batchId", batchID, "fileStorageId", sid)
		}
	}
	// set pending gauge for the batch
	if s.batchRepo != nil {
		metrics.BatchJobsPending.WithLabelValues(batchID).Set(float64(validCount))
	}
	if s.batchRepo != nil {
		_ = s.batchRepo.Update(ctx, b)
	}

	// update last update timestamp after persisting batch
	metrics.BatchLastUpdateTimestamp.WithLabelValues(batchID).Set(float64(time.Now().Unix()))

	if invalidCount > 0 {
		if s.Logger != nil {
			traceID := ""
			if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
				traceID = fmt.Sprintf("%v", sc.TraceID())
			}
			s.Logger.Warn("validation_issues", "traceId", traceID, "batchId", batchID, "invalidRows", invalidCount, "totalRows", total)
		}
	}

	return &CreateBatchResult{BatchID: batchID, TotalRows: total, ValidRows: validCount, InvalidRows: invalidCount, Errors: validationErrors}, nil
}

// ConfirmBatch blocks billing and enqueues jobs — simplified implementation
func (s *Service) ConfirmBatch(ctx context.Context, batchID string, count int, blockFunc func(context.Context, string, int, string) (interface{}, error), publishFunc func(context.Context, string, []byte, any) error) (int, error) {
	// Fetch batch and validate requested count against approved allowance (partial flow)
	if s.batchRepo != nil {
		if bObj, berr := s.batchRepo.GetByID(ctx, batchID); berr == nil && bObj != nil {
			// If ApprovedCount was set by BatchManager during FinalizeAfterUpload (partial available),
			// disallow confirming more than approved.
			if bObj.ApprovedCount > 0 && count > bObj.ApprovedCount {
				return 0, fmt.Errorf("requested count %d exceeds approved allowedTotal %d", count, bObj.ApprovedCount)
			}
		}
	}
	// call billing to reserve transactions and collect transaction IDs
	var txIDs []string
	// attempt to call provided blockFunc (test hook) first
	if blockFunc != nil {
		if s.Logger != nil {
			s.Logger.Info("billing_call_started", "batchId", batchID, "count", count)
		}
		res, err := blockFunc(ctx, "", count, batchID)
		if err != nil {
			metrics.BillingErrorsTotal.WithLabelValues("BlockBatch", "error").Inc()
			if s.Logger != nil {
				s.Logger.Error("billing_call_failed", "batchId", batchID, "error", err)
			}
			return 0, err
		}
		metrics.BillingCallsTotal.WithLabelValues("BlockBatch", "success").Inc()
		// try to extract TransactionIDs from common types
		if br, ok := res.(*billing.BlockBatchResponse); ok && br != nil {
			txIDs = br.TransactionIDs
		} else if br2, ok := res.(billing.BlockBatchResponse); ok {
			txIDs = br2.TransactionIDs
		} else if m, ok := res.(map[string]interface{}); ok {
			if t, ok := m["transactionIds"]; ok {
				switch vt := t.(type) {
				case []string:
					txIDs = vt
				case []interface{}:
					for _, it := range vt {
						if sstr, ok := it.(string); ok {
							txIDs = append(txIDs, sstr)
						}
					}
				}
			}
		}
		if s.Logger != nil {
			s.Logger.Info("billing_call_succeeded", "batchId", batchID, "txCount", len(txIDs))
		}
		// Persist transaction IDs on batch for audit and possible rollback (TZ §9.2)
		if len(txIDs) > 0 && s.batchRepo != nil {
			if bObj, berr := s.batchRepo.GetByID(ctx, batchID); berr == nil && bObj != nil {
				bObj.BillingTransactionIDs = txIDs
				_ = s.batchRepo.Update(ctx, bObj)
			}
		}
	} else if s.billing != nil {
		if s.Logger != nil {
			s.Logger.Info("billing_call_started", "batchId", batchID, "count", count)
		}
		br, err := s.billing.BlockBatch(ctx, "", count, batchID)
		if err != nil {
			metrics.BillingErrorsTotal.WithLabelValues("BlockBatch", "error").Inc()
			if s.Logger != nil {
				s.Logger.Error("billing_call_failed", "batchId", batchID, "error", err)
			}
			return 0, err
		}
		metrics.BillingCallsTotal.WithLabelValues("BlockBatch", "success").Inc()
		if br != nil {
			txIDs = br.TransactionIDs
		}
		if s.Logger != nil {
			s.Logger.Info("billing_call_succeeded", "batchId", batchID, "txCount", len(txIDs))
		}
	}

	// fetch jobs and publish up to count; assign tx ids to jobs where possible
	queued := 0
	publishError := false
	var assignedTxIDs []string
	if s.jobRepo != nil {
		jobs, err := s.jobRepo.GetByBatch(ctx, batchID)
		if err != nil {
			return 0, err
		}

		// helper to assign tx for job index
		assignTx := func(i int) *string {
			if len(txIDs) == 0 {
				return nil
			}
			if len(txIDs) == 1 {
				return &txIDs[0]
			}
			if len(txIDs) >= count && len(txIDs) == count {
				return &txIDs[i]
			}
			if i < len(txIDs) {
				return &txIDs[i]
			}
			return nil
		}

		// Попробуем получить информацию о batch (userId, revision) чтобы положить в сообщение согласно п.7.2 ТЗ
		var batchUserID string
		var batchRevision string
		if s.batchRepo != nil {
			if bObj, berr := s.batchRepo.GetByID(ctx, batchID); berr == nil && bObj != nil {
				batchUserID = bObj.UserID
				batchRevision = bObj.Revision
			}
		}

		for i, j := range jobs {
			if i >= count {
				break
			}
			tx := assignTx(i)
			// persist job-level billing_transaction_id only if valid UUID
			if tx != nil {
				if _, perr := uuid.Parse(*tx); perr == nil {
					_ = s.jobRepo.UpdateBillingTransactionID(ctx, j.ID, tx)
					assignedTxIDs = append(assignedTxIDs, *tx)
				} else {
					if s.Logger != nil {
						s.Logger.Warn("invalid_billing_txid_format", "batchId", batchID, "jobId", j.ID)
					}
				}
			}

			// Формируем сообщение согласно п.7.2 ТЗ:
			// {
			//   "userId": "uuid-user-id",
			//   "batchId": "uuid-batch-id",
			//   "buildId": "uuid-build-id",
			//   "rowNumber": 5,
			//   "revision": "US_CA_08292017",
			//   "fields": { ... },
			//   "billingPreApproved": true,
			//   "transactionId": "uuid-tx-id"
			// }
			// Примечание: поле buildId в момент публикации задач обычно ещё не заполнено — оставляем пустым (BFF/consumer может заполнить при обработке).
			// build base event according to TZ §8.2 / §7.2
			evt := map[string]any{
				"eventType":          "bulk.job",
				"jobId":              j.ID,
				"batchId":            j.BatchID,
				"userId":             batchUserID,
				"buildId":            j.ID, // заполняем buildId значением jobId для совместимости с п.7.2 ТЗ
				"rowNumber":          j.RowNumber,
				"revision":           batchRevision,
				"fields":             map[string]string{},
				"billingPreApproved": false,
				// timestamp as RFC3339 to comply with TZ example
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			}
			// если у job в InputData сохранены сериализованные поля — попытаемся их распарсить и вставить в fields
			if j.InputData != "" {
				var fm map[string]string
				if uerr := json.Unmarshal([]byte(j.InputData), &fm); uerr == nil {
					evt["fields"] = fm
				}
			}
			if tx != nil {
				evt["billingPreApproved"] = true
				evt["transactionId"] = *tx
				// для обратной совместимости также добавляем billingTransactionId
				evt["billingTransactionId"] = *tx
			}

			// metadata: include source and correlationId (trace id) per TZ §8.2 example
			if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
				evt["metadata"] = map[string]any{"source": "bulk-service", "correlationId": sc.TraceID().String()}
			} else {
				// fallback correlationId: generate a uuid to help tracing
				evt["metadata"] = map[string]any{"source": "bulk-service", "correlationId": uuid.New().String()}
			}

			// log publishing attempt
			if s.Logger != nil {
				traceID := ""
				if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
					traceID = sc.TraceID().String()
				}
				s.Logger.Info("publishing_job", "traceId", traceID, "jobId", j.ID, "batchId", j.BatchID)
			}
			// publish to configured topic (fallback to "bulk.job")
			topic := os.Getenv("KAFKA_TOPIC_BULK_JOB")
			if topic == "" {
				topic = "bulk.job"
			}
			var perr error
			if publishFunc != nil {
				perr = publishFunc(ctx, topic, nil, evt)
			} else if s.producer != nil {
				perr = s.producer.Publish(ctx, topic, nil, evt)
			}

			if perr != nil {
				publishError = true
				if s.Logger != nil {
					s.Logger.Error("publishing_failed", "batchId", batchID, "jobId", j.ID, "error", perr)
				}
				// do not mark this job as queued
			} else {
				// update status to queued
				if err := s.jobRepo.UpdateStatus(ctx, j.ID, string(batch.JobStatusQueued)); err == nil {
					queued++
					metrics.BatchJobsPending.WithLabelValues(batchID).Dec()
					metrics.BatchJobsTotal.WithLabelValues(batchID, string(batch.JobStatusQueued)).Inc()
					if s.Logger != nil {
						traceID := ""
						if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
							traceID = fmt.Sprintf("%v", sc.TraceID())
						}
						// DEBUG level event for queued jobs per TZ §13.2
						s.Logger.Debug("job_queued", "traceId", traceID, "batchId", j.BatchID, "jobId", j.ID, "rowNumber", j.RowNumber)
						// also keep existing info-level status update for compatibility
						s.Logger.Info("job_status_updated", "traceId", traceID, "jobId", j.ID, "batchId", j.BatchID, "status", "queued")
					}
				}
			}
		}

		// on publish error -> attempt refund of assigned tx ids and mark batch/jobs
		if publishError {
			if len(assignedTxIDs) > 0 && s.billing != nil {
				_ = s.billing.RefundTransactions(ctx, "", assignedTxIDs, batchID)
			}
			// mark batch cancelled
			if s.batchRepo != nil {
				if b, err := s.batchRepo.GetByID(ctx, batchID); err == nil && b != nil {
					b.Status = domain.BatchStatusCancelled
					_ = s.batchRepo.Update(ctx, b)
				}
			}
			// mark unqueued jobs as refunded
			for i := queued; i < len(jobs) && i < count; i++ {
				_ = s.jobRepo.UpdateStatus(ctx, jobs[i].ID, string(batch.JobStatusRefunded))
			}
		}
	}

	return queued, nil
}

// ExportResultsXLS generates XLSX bytes for results of a batch and returns a ReadCloser
func (s *Service) ExportResultsXLS(ctx context.Context, batchID string, limit int) (io.ReadCloser, error) {
	if s.jobRepo == nil {
		return nil, nil
	}
	results, err := s.jobRepo.GetResultsByBatch(ctx, batchID)
	if err != nil {
		return nil, err
	}

	f := excelize.NewFile()
	sheet := "Results"
	if _, err := f.NewSheet(sheet); err != nil {
		return nil, err
	}
	sw, err := f.NewStreamWriter(sheet)
	if err != nil {
		return nil, err
	}
	headers := []interface{}{"row", "buildId", "pdf417_url", "code128_url", "error_code", "error_message"}
	if err := sw.SetRow("A1", headers); err != nil {
		return nil, err
	}

	rowIndex := 2
	for i, res := range results {
		if limit > 0 && i >= limit {
			break
		}
		pdf, code128 := pkgxls.ParseBarcodeURLs(res.BarcodeURLs)
		vals := []interface{}{res.RowNumber, res.BuildID, pdf, code128, res.ErrorCode, res.ErrorMessage}
		cell, _ := excelize.CoordinatesToCellName(1, rowIndex)
		if err := sw.SetRow(cell, vals); err != nil {
			return nil, err
		}
		rowIndex++
	}
	if err := sw.Flush(); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(buf.Bytes())), nil
}
