package database

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Connection besitzt den einzigen PostgreSQL-Pool des API-Prozesses.
// ORM wird in späteren Schritten an Repositories übergeben; SQL hält bestehende
// Repository- und Migrationsschnittstellen während der Umstellung funktionsfähig.
type Connection struct {
	ORM *gorm.DB
	SQL *sql.DB
}

func (connection *Connection) Close() error {
	return connection.SQL.Close()
}

// OpenGORMFromEnv bindet GORM an denselben pgx/database/sql-Pool.
// SQL-Migrationen bleiben die einzige Quelle für Schemaänderungen.
func OpenGORMFromEnv(logger *slog.Logger) (*Connection, error) {
	if logger == nil {
		return nil, errors.New("Logger fehlt")
	}
	sqlDB, err := openPoolFromEnv()
	if err != nil {
		return nil, err
	}
	orm, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		TranslateError:       true,
		DisableAutomaticPing: true,
		Logger:               newSafeGORMLogger(logger),
	})
	if err != nil {
		sqlDB.Close()
		return nil, errors.New("GORM konnte nicht initialisiert werden")
	}
	pool, err := orm.DB()
	if err != nil || pool != sqlDB {
		sqlDB.Close()
		return nil, errors.New("GORM verwendet nicht den erwarteten PostgreSQL-Pool")
	}
	configurePool(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.PingContext(ctx); err != nil {
		pool.Close()
		return nil, errors.New("PostgreSQL ist nicht erreichbar")
	}
	return &Connection{ORM: orm, SQL: pool}, nil
}
