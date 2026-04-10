package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Server struct {
		Port         int           `env:"SERVER_PORT" envDefault:"8080"`
		Host         string        `env:"SERVER_HOST" envDefault:"0.0.0.0"`
		ReadTimeout  time.Duration `env:"SERVER_READ_TIMEOUT" envDefault:"30s"`
		WriteTimeout time.Duration `env:"SERVER_WRITE_TIMEOUT" envDefault:"60s"`
	}

	// Service metadata and logging configuration (соответствует TZ §13.1 и удобству эксплуатации)
	Service struct {
		// Name used in structured logs (env: SERVICE_NAME)
		Name string `env:"SERVICE_NAME" envDefault:"bulk-service"`
		// Version used in structured logs (env: SERVICE_VERSION)
		Version string `env:"SERVICE_VERSION" envDefault:"unknown"`
	}

	Log struct {
		// Log level: debug, info, warn, error (env: LOG_LEVEL)
		Level string `env:"LOG_LEVEL" envDefault:"info"`
		// Log format: json or console (env: LOG_FORMAT)
		Format string `env:"LOG_FORMAT" envDefault:"json"`
	}

	BFF struct {
		URL           string        `env:"BFF_URL,required"`
		Timeout       time.Duration `env:"BFF_TIMEOUT" envDefault:"30s"`
		RetryAttempts int           `env:"BFF_RETRY_ATTEMPTS" envDefault:"3"`
		// ServiceToken — служебный токен для внутренних вызовов BFF (/internal/*).
		// Примечание: в оригинальном Bulk_Service_TZ явно не было указания
		// об обязательной передаче service token. Однако BFF защищает internal
		// маршруты service token (п.16.1 ТЗ BFF). Поэтому Bulk загружает
		// этот токен для возможности авторизованных внутренних вызовов.
		ServiceToken string `env:"BFF_SERVICE_TOKEN"`
	}

	Kafka struct {
		Brokers         string `env:"KAFKA_BROKERS" envDefault:"localhost:9092"`
		BulkJobTopic    string `env:"KAFKA_TOPIC_BULK_JOB" envDefault:"bulk.job"`
		BulkResultTopic string `env:"KAFKA_TOPIC_BULK_RESULT" envDefault:"bulk.result"`
		// BulkStatusTopic — topic for publishing batch status updates (TZ §8.1)
		BulkStatusTopic string `env:"KAFKA_TOPIC_BULK_STATUS" envDefault:"bulk.status"`
		ConsumerGroup   string `env:"KAFKA_CONSUMER_GROUP" envDefault:"bulk-service"`
	}

	Database struct {
		URL           string        `env:"DATABASE_URL"`
		StatsInterval time.Duration `env:"DB_STATS_INTERVAL"`
		// MaxOpenConns соответствует TZ 12.2: позволяет конфигурировать
		// максимальное число открытых соединений к БД (env: DATABASE_MAX_OPEN_CONNS).
		// Если не задано, используется значение по умолчанию 25.
		MaxOpenConns int `env:"DATABASE_MAX_OPEN_CONNS" envDefault:"25"`
		// MaxIdleConns позволяет задать число неактивных (idle) соединений в пуле.
		// Соответствует переменной окружения DATABASE_MAX_IDLE_CONNS (по умолчанию 5).
		MaxIdleConns int `env:"DATABASE_MAX_IDLE_CONNS" envDefault:"5"`
	}

	// Redis — используется для rate limiting и idempotency keys (ТЗ §12.1)
	Redis struct {
		URL      string `env:"REDIS_URL" envDefault:"redis://redis:6379/0"`
		PoolSize int    `env:"REDIS_POOL_SIZE" envDefault:"10"`
	}

	Limits struct {
		MaxFileSizeMB        int `env:"MAX_FILE_SIZE_MB" envDefault:"10"`
		MaxRowsPerBatch      int `env:"MAX_ROWS_PER_BATCH" envDefault:"1000"`
		MaxConcurrentBatches int `env:"MAX_CONCURRENT_BATCHES" envDefault:"5"`
		// MaxBatchesPerHour соответствует TZ 12.2: ограничение числа батчей в час
		MaxBatchesPerHour int `env:"MAX_BATCHES_PER_HOUR" envDefault:"10"`
	}
}

func LoadConfigFromEnv() (*Config, error) {
	cfg := &Config{}
	// Server
	if v := os.Getenv("SERVER_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = p
		}
	} else {
		cfg.Server.Port = 8080
	}
	if v := os.Getenv("SERVER_HOST"); v != "" {
		cfg.Server.Host = v
	} else {
		cfg.Server.Host = "0.0.0.0"
	}
	if v := os.Getenv("SERVER_READ_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Server.ReadTimeout = d
		} else {
			cfg.Server.ReadTimeout = 30 * time.Second
		}
	} else {
		cfg.Server.ReadTimeout = 30 * time.Second
	}
	if v := os.Getenv("SERVER_WRITE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Server.WriteTimeout = d
		} else {
			cfg.Server.WriteTimeout = 60 * time.Second
		}
	} else {
		cfg.Server.WriteTimeout = 60 * time.Second
	}

	// Service metadata and logging defaults
	if v := os.Getenv("SERVICE_NAME"); v != "" {
		cfg.Service.Name = v
	} else {
		cfg.Service.Name = "bulk-service"
	}
	if v := os.Getenv("SERVICE_VERSION"); v != "" {
		cfg.Service.Version = v
	} else {
		cfg.Service.Version = "unknown"
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.Log.Level = v
	} else {
		cfg.Log.Level = "info"
	}
	if v := os.Getenv("LOG_FORMAT"); v != "" {
		cfg.Log.Format = v
	} else {
		cfg.Log.Format = "json"
	}

	// BFF
	cfg.BFF.URL = os.Getenv("BFF_URL")
	if v := os.Getenv("BFF_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.BFF.Timeout = d
		} else {
			cfg.BFF.Timeout = 30 * time.Second
		}
	} else {
		cfg.BFF.Timeout = 30 * time.Second
	}
	if v := os.Getenv("BFF_RETRY_ATTEMPTS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.BFF.RetryAttempts = n
		} else {
			cfg.BFF.RetryAttempts = 3
		}
	} else {
		cfg.BFF.RetryAttempts = 3
	}
	cfg.BFF.ServiceToken = os.Getenv("BFF_SERVICE_TOKEN")

	// Kafka
	if v := os.Getenv("KAFKA_BROKERS"); v != "" {
		cfg.Kafka.Brokers = v
	}

	// Database
	cfg.Database.URL = os.Getenv("DATABASE_URL")
	if v := os.Getenv("DB_STATS_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Database.StatsInterval = d
		} else {
			cfg.Database.StatsInterval = 10 * time.Second
		}
	} else {
		cfg.Database.StatsInterval = 10 * time.Second
	}
	if v := os.Getenv("DATABASE_MAX_OPEN_CONNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Database.MaxOpenConns = n
		} else {
			cfg.Database.MaxOpenConns = 25
		}
	} else {
		cfg.Database.MaxOpenConns = 25
	}
	if v := os.Getenv("DATABASE_MAX_IDLE_CONNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Database.MaxIdleConns = n
		} else {
			cfg.Database.MaxIdleConns = 5
		}
	} else {
		cfg.Database.MaxIdleConns = 5
	}

	// Redis (ТЗ §12.1: REDIS_URL, REDIS_POOL_SIZE)
	if v := os.Getenv("REDIS_URL"); v != "" {
		cfg.Redis.URL = v
	} else {
		cfg.Redis.URL = "redis://redis:6379/0"
	}
	if v := os.Getenv("REDIS_POOL_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Redis.PoolSize = n
		} else {
			cfg.Redis.PoolSize = 10
		}
	} else {
		cfg.Redis.PoolSize = 10
	}

	// Limits
	if v := os.Getenv("MAX_FILE_SIZE_MB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Limits.MaxFileSizeMB = n
		}
	}
	if v := os.Getenv("MAX_BATCHES_PER_HOUR"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Limits.MaxBatchesPerHour = n
		}
	} else {
		cfg.Limits.MaxBatchesPerHour = 10
	}

	return cfg, nil
}
