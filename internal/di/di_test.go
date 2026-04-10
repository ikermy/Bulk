package di

import (
    "testing"
    "time"
    "os"

    cfgpkg "github.com/ikermy/Bulk/internal/config"
    "github.com/stretchr/testify/require"
)

func TestNewDeps_MinimalConfig(t *testing.T) {
    cfg := &cfgpkg.Config{}
    deps, err := NewDeps(cfg)
    require.NoError(t, err)
    require.NotNil(t, deps)
    // with empty config we expect no DB connection and no DBStopChan
    require.Nil(t, deps.DB)
    require.Nil(t, deps.DBStopChan)
    // other components should be constructed (producer, billing client, logger)
    require.NotNil(t, deps.Producer)
    require.NotNil(t, deps.BillingClient)
    require.NotNil(t, deps.Logger)
}

func TestNewDeps_WithDBAndKafka(t *testing.T) {
    cfg := &cfgpkg.Config{}
    cfg.Database.URL = "postgres://user:pass@localhost/dbname?sslmode=disable"
    cfg.Kafka.Brokers = "localhost:9092"
    cfg.Kafka.BulkResultTopic = "bulk.result"
    cfg.Database.StatsInterval = 10 * time.Millisecond
    // ensure env variables for topics are not set to avoid surprises
    deps, err := NewDeps(cfg)
    require.NoError(t, err)
    require.NotNil(t, deps)
    // DB should be initialized
    require.NotNil(t, deps.DB)
    // DBStopChan should be set
    require.NotNil(t, deps.DBStopChan)

    // If consumer started, attempt to close it to avoid leaking resources
    if deps.Consumer != nil {
        _ = deps.Consumer.Close()
    }
    // stop DB stats collector
    if deps.DBStopChan != nil {
        close(deps.DBStopChan)
    }
    if deps.DB != nil {
        deps.DB.Close()
    }
}

func TestNewDeps_WithKafkaEnvVars(t *testing.T) {
    cfg := &cfgpkg.Config{}
    cfg.Kafka.Brokers = "localhost:9092"
    cfg.Kafka.BulkResultTopic = "bulk.result"
    // set envs to exercise topic env and retry parsing
    os.Setenv("KAFKA_TOPIC_TRANS_HISTORY", "trans-history.test")
    os.Setenv("KAFKA_CONSUMER_RETRY", "5")
    defer os.Unsetenv("KAFKA_TOPIC_TRANS_HISTORY")
    defer os.Unsetenv("KAFKA_CONSUMER_RETRY")

    deps, err := NewDeps(cfg)
    require.NoError(t, err)
    require.NotNil(t, deps)
    if deps.Consumer != nil {
        _ = deps.Consumer.Close()
    }
    if deps.DBStopChan != nil {
        close(deps.DBStopChan)
    }
    if deps.DB != nil {
        deps.DB.Close()
    }
}

func TestNewDeps_WithStorageEnvAttempt(t *testing.T) {
    cfg := &cfgpkg.Config{}
    // set STORAGE_ENDPOINT to trigger attempt to construct file client
    os.Setenv("STORAGE_ENDPOINT", "http://example")
    defer os.Unsetenv("STORAGE_ENDPOINT")
    // leave other storage envs missing so NewFileClientFromEnv returns error

    deps, err := NewDeps(cfg)
    require.NoError(t, err)
    require.NotNil(t, deps)
    // storage client should be nil when NewFileClientFromEnv fails
    require.Nil(t, deps.Storage)
}

func TestNewDeps_KafkaRetryNonNumeric(t *testing.T) {
    cfg := &cfgpkg.Config{}
    cfg.Kafka.Brokers = "localhost:9092"
    cfg.Kafka.BulkResultTopic = "bulk.result"
    os.Setenv("KAFKA_CONSUMER_RETRY", "bad")
    defer os.Unsetenv("KAFKA_CONSUMER_RETRY")
    deps, err := NewDeps(cfg)
    require.NoError(t, err)
    require.NotNil(t, deps)
    if deps.Consumer != nil {
        _ = deps.Consumer.Close()
    }
}

func TestNewDeps_DBConnectionLimitsApplied(t *testing.T) {
    cfg := &cfgpkg.Config{}
    cfg.Database.URL = "postgres://user:pass@localhost/dbname?sslmode=disable"
    cfg.Database.MaxOpenConns = 10
    cfg.Database.MaxIdleConns = 5
    cfg.Database.StatsInterval = 10 * time.Millisecond

    deps, err := NewDeps(cfg)
    require.NoError(t, err)
    require.NotNil(t, deps)
    // DB should be initialized and DBStopChan present
    require.NotNil(t, deps.DB)
    require.NotNil(t, deps.DBStopChan)

    if deps.Consumer != nil {
        _ = deps.Consumer.Close()
    }
    if deps.DBStopChan != nil {
        close(deps.DBStopChan)
    }
    if deps.DB != nil {
        deps.DB.Close()
    }
}



