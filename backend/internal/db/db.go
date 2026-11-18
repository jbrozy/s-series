// Package db opens the Postgres connection and applies migrations.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

// Open connects with retries (the database container may still be starting).
func Open(ctx context.Context, url string) (*sql.DB, error) {
	var lastErr error
	for i := 0; i < 30; i++ {
		db, err := sql.Open("postgres", url)
		if err == nil {
			db.SetMaxOpenConns(20)
			db.SetMaxIdleConns(5)
			db.SetConnMaxIdleTime(5 * time.Minute)
			pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
			err = db.PingContext(pctx)
			cancel()
			if err == nil {
				return db, nil
			}
			db.Close()
		}
		lastErr = err
		log.Printf("Datenbank noch nicht erreichbar (%v) – neuer Versuch …", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return nil, fmt.Errorf("Datenbank nicht erreichbar: %w", lastErr)
}

// Migrate applies all *.sql files in dir that were not applied yet.
func Migrate(ctx context.Context, db *sql.DB, dir string) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("Migrationsverzeichnis %s: %w", dir, err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names {
		var exists bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name=$1)`, n).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			return err
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("Migration %s: %w", n, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(name) VALUES ($1)`, n); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		log.Printf("Migration %s angewendet", n)
	}
	return nil
}
