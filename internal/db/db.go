package db

import (
	"database/sql"
	"time"

	"github.com/ikermy/Bulk/internal/metrics"
	_ "github.com/lib/pq" // register postgres driver
)

func Connect(databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	return db, nil
}

// StartDBStatsCollector periodically reads sql.DB.Stats() and updates Prometheus gauges.
// Caller should cancel ctx to stop the collector.
func StartDBStatsCollector(db *sql.DB, interval time.Duration, stop <-chan struct{}) {
	if db == nil {
		return
	}
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				stats := db.Stats()
				metrics.DBOpenConnections.Set(float64(stats.OpenConnections))
				metrics.DBIdleConnections.Set(float64(stats.Idle))
			case <-stop:
				return
			}
		}
	}()
}
