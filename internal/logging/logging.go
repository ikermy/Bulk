package logging

import (
	"os"

	"github.com/ikermy/Bulk/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger = *zap.SugaredLogger

// NewLogger создает JSON-логгер, совместимый с требованиями ТЗ §13.1.
// - формат: JSON
// - ключи: timestamp, level, message
// - время в ISO8601 (UTC)
// Также добавляет сервисные поля `service` и `version` из окружения
// (SERVICE_NAME, SERVICE_VERSION) для соответствия образцу.
func NewLogger(cfg *config.Config) Logger {
	// handle nil cfg gracefully
	var svcName, svcVer, logLevel, logFormat string
	if cfg != nil {
		svcName = cfg.Service.Name
		svcVer = cfg.Service.Version
		logLevel = cfg.Log.Level
		logFormat = cfg.Log.Format
	}
	// fallback to env/defaults when cfg omitted or empty
	if svcName == "" {
		svcName = os.Getenv("SERVICE_NAME")
		if svcName == "" {
			svcName = "bulk-service"
		}
	}
	if svcVer == "" {
		svcVer = os.Getenv("SERVICE_VERSION")
		if svcVer == "" {
			svcVer = "unknown"
		}
	}
	if logLevel == "" {
		logLevel = os.Getenv("LOG_LEVEL")
		if logLevel == "" {
			logLevel = "info"
		}
	}
	if logFormat == "" {
		logFormat = os.Getenv("LOG_FORMAT")
		if logFormat == "" {
			logFormat = "json"
		}
	}

	zcfg := zap.NewProductionConfig()
	// encoder keys: приводим к ожидаемым именам: timestamp, level, message
	zcfg.EncoderConfig.TimeKey = "timestamp"
	zcfg.EncoderConfig.LevelKey = "level"
	zcfg.EncoderConfig.NameKey = "logger"
	zcfg.EncoderConfig.CallerKey = "caller"
	zcfg.EncoderConfig.MessageKey = "message"
	zcfg.EncoderConfig.StacktraceKey = "stacktrace"
	zcfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	// set level
	var lvl zapcore.Level
	if err := lvl.UnmarshalText([]byte(logLevel)); err == nil {
		zcfg.Level = zap.NewAtomicLevelAt(lvl)
	} else {
		// default to info
		zcfg.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	}

	// set encoder format
	if logFormat == "console" {
		zcfg.Encoding = "console"
	} else {
		zcfg.Encoding = "json"
	}

	base, _ := zcfg.Build()
	base = base.With(zap.String("service", svcName), zap.String("version", svcVer))
	return base.Sugar()
}
