package database

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// safeGORMLogger protokolliert bewusst weder SQL noch Bind-Parameter oder Fehlermeldungen.
type safeGORMLogger struct {
	logger        *slog.Logger
	level         gormlogger.LogLevel
	slowThreshold time.Duration
}

func newSafeGORMLogger(logger *slog.Logger) gormlogger.Interface {
	return safeGORMLogger{logger: logger, level: gormlogger.Warn, slowThreshold: time.Second}
}

func (log safeGORMLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	log.level = level
	return log
}

func (log safeGORMLogger) Info(ctx context.Context, _ string, _ ...interface{}) {
	if log.level >= gormlogger.Info {
		log.logger.InfoContext(ctx, "database.gorm_info")
	}
}

func (log safeGORMLogger) Warn(ctx context.Context, _ string, _ ...interface{}) {
	if log.level >= gormlogger.Warn {
		log.logger.WarnContext(ctx, "database.gorm_warning")
	}
}

func (log safeGORMLogger) Error(ctx context.Context, _ string, _ ...interface{}) {
	if log.level >= gormlogger.Error {
		log.logger.ErrorContext(ctx, "database.gorm_error")
	}
}

func (log safeGORMLogger) Trace(ctx context.Context, begin time.Time, _ func() (string, int64), err error) {
	if log.level <= gormlogger.Silent {
		return
	}
	duration := time.Since(begin)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) &&
		!errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		if log.level >= gormlogger.Error {
			log.logger.ErrorContext(ctx, "database.query_failed", "duration_ms", duration.Milliseconds())
		}
		return
	}
	if duration > log.slowThreshold && log.level >= gormlogger.Warn {
		log.logger.WarnContext(ctx, "database.query_slow", "duration_ms", duration.Milliseconds())
	} else if log.level >= gormlogger.Info {
		log.logger.InfoContext(ctx, "database.query", "duration_ms", duration.Milliseconds())
	}
}
