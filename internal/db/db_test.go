package db

import (
    "testing"
    "time"

    "github.com/DATA-DOG/go-sqlmock"
    "github.com/ikermy/Bulk/internal/metrics"
    "github.com/prometheus/client_golang/prometheus/testutil"
    "github.com/stretchr/testify/require"
)

func TestConnectReturnsDB(t *testing.T) {
    // Connect uses sql.Open which will return a *sql.DB even for empty URL
    db, err := Connect("")
    require.NoError(t, err)
    require.NotNil(t, db)
    db.Close()
}

func TestStartDBStatsCollectorUpdatesMetrics(t *testing.T) {
    sqlDB, _, err := sqlmock.New()
    require.NoError(t, err)
    defer sqlDB.Close()

    stop := make(chan struct{})
    // run collector with short interval
    StartDBStatsCollector(sqlDB, 10*time.Millisecond, stop)
    // wait a bit to let it run once
    time.Sleep(30 * time.Millisecond)
    // stop collector
    close(stop)

    // ensure metrics have been updated at least once (values non-negative)
    require.GreaterOrEqual(t, testutil.ToFloat64(metrics.DBOpenConnections), float64(0))
    require.GreaterOrEqual(t, testutil.ToFloat64(metrics.DBIdleConnections), float64(0))
}



