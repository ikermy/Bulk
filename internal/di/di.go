package di

import (
	"context"
	"database/sql"
	"os"
	"strconv"

	"github.com/ikermy/Bulk/internal/batch"
	"github.com/ikermy/Bulk/internal/billing"
	cfg "github.com/ikermy/Bulk/internal/config"
	"github.com/ikermy/Bulk/internal/db"
	"github.com/ikermy/Bulk/internal/exporter"
	"github.com/ikermy/Bulk/internal/history"
	"github.com/ikermy/Bulk/internal/kafka"
	"github.com/ikermy/Bulk/internal/logging"
	"github.com/ikermy/Bulk/internal/ports"
	"github.com/ikermy/Bulk/internal/repo"
	"github.com/ikermy/Bulk/internal/storage"
	svc "github.com/ikermy/Bulk/internal/usecase/bulk"
	"github.com/ikermy/Bulk/internal/validation"
)

type Deps struct {
	Logger         logging.Logger
	BatchRepo      ports.BatchRepository
	JobRepo        ports.JobRepository
	ValidationRepo ports.ValidationRepository
	BatchManager   *batch.BatchManager
	BillingClient  ports.BillingClient
	Producer       ports.KafkaProducer
	DB             *sql.DB
	Consumer       *kafka.ResultConsumer
	History        *history.Tagger
	Service        *svc.Service
	DBStopChan     chan struct{}
	Storage        storage.Client
	ExportManager  exporter.ManagerAPI
}

// NewDeps constructs all dependencies from config: db, repos, clients, producer and service
func NewDeps(cfg *cfg.Config) (*Deps, error) {
	var dbConn *sql.DB
	var err error
	if cfg.Database.URL != "" {
		dbConn, err = db.Connect(cfg.Database.URL)
		if err != nil {
			return nil, err
		}
		// apply configured DB connection limits from config (соответствует TZ 12.2)
		if cfg.Database.MaxOpenConns > 0 {
			dbConn.SetMaxOpenConns(cfg.Database.MaxOpenConns)
		}
		if cfg.Database.MaxIdleConns > 0 {
			dbConn.SetMaxIdleConns(cfg.Database.MaxIdleConns)
		}
	}

	// start DB stats collector if DB available
	var dbStop chan struct{}
	if dbConn != nil {
		dbStop = make(chan struct{})
		db.StartDBStatsCollector(dbConn, cfg.Database.StatsInterval, dbStop)
	}

	batchRepo := repo.NewBatchRepository(dbConn)
	jobRepo := repo.NewJobRepository(dbConn)
	var validationRepo ports.ValidationRepository
	if dbConn != nil {
		validationRepo = repo.NewValidationRepository(dbConn)
	}

	// Pass BFF service token to billing client so internal billing calls include Authorization header.
	// Note: Bulk_Service_TZ.txt didn't explicitly require Bulk to send a service token, but BFF
	// protects /internal/* routes with ServiceJWTMiddleware (p.16.1 in BFF TZ), so we provide
	// the token from configuration to be compatible with secured BFF deployments.
	billingClient := billing.NewBFFBillingClient(cfg.BFF.URL, cfg.BFF.Timeout, cfg.BFF.ServiceToken)

	producer := kafka.NewProducer(cfg.Kafka.Brokers)
	// history tagger - publish to trans-history.log topic per TZ
	histTopic := os.Getenv("KAFKA_TOPIC_TRANS_HISTORY")
	if histTopic == "" {
		histTopic = "trans-history.log"
	}
	hist := history.NewTagger(producer, histTopic)
	// initialize logger early so it can be passed to components
	logger := logging.NewLogger(cfg)
	// create batch manager early so consumer handler can notify batch state changes
	var batchManager *batch.BatchManager
	if batchRepo != nil && jobRepo != nil {
		// pass producer and configured bulk status topic so BatchManager can publish bulk.status per TZ §8.1
		batchManager = batch.NewBatchManager(batchRepo, jobRepo, billingClient, hist, producer, cfg.Kafka.BulkStatusTopic)
	}
	// start a result consumer in background (if brokers provided)
	var consumer *kafka.ResultConsumer
	if cfg.Kafka.Brokers != "" {
		dlq := os.Getenv("KAFKA_DLQ_TOPIC")
		if dlq == "" {
			dlq = "bulk.result.dlq"
		}
		retry := 3
		if v := os.Getenv("KAFKA_CONSUMER_RETRY"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				retry = n
			}
		}
		// handler for processing BulkResultEvent — implements TZ §8.3.
		// Extracted to buildResultHandler for testability (see di_handler.go).
		handler := buildResultHandler(jobRepo, hist, batchManager, logger)
		consumer = kafka.NewConsumer(cfg.Kafka.Brokers, cfg.Kafka.BulkResultTopic, cfg.Kafka.ConsumerGroup, handler, producer, dlq, retry, logger)
		go func() {
			_ = consumer.Start(context.Background())
		}()
	}

	// NOTE: BFF requires service token for /internal/* routes (see BFF TЗ p.16.1).
	// Bulk_Service_TZ does not explicitly mention this token requirement for Bulk,
	// поэтому мы передаём service token из конфигурации чтобы клиент мог его использовать.
	validator := validation.NewBFFValidator(cfg.BFF.URL, cfg.BFF.Timeout, cfg.BFF.ServiceToken)

	// try to initialize storage client (optional)
	var storageClient *storage.FileClient
	if os.Getenv("STORAGE_ENDPOINT") != "" {
		sc, serr := storage.NewFileClientFromEnv()
		if serr == nil {
			storageClient = sc
		}
	}

	service := svc.NewService(batchRepo, jobRepo, validator, billingClient, producer, storageClient)
	if service != nil {
		service.Logger = logger
	}
	// propagate logger to batch manager if present
	if batchManager != nil {
		batchManager.Logger = logger
	}

	// initialize exporter manager with small buffer
	var expManager *exporter.Manager
	expManager = exporter.NewManager(10, storageClient, service)
	if expManager != nil {
		expManager.Logger = logger
	}

	deps := &Deps{Logger: logger, BatchRepo: batchRepo, JobRepo: jobRepo, ValidationRepo: validationRepo, BatchManager: batchManager, BillingClient: billingClient, Producer: producer, DB: dbConn, Consumer: consumer, History: hist, Service: service, DBStopChan: dbStop, Storage: storageClient, ExportManager: expManager}
	return deps, nil
}
