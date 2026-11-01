// Package testdb gives integration tests an isolated, fully migrated Postgres
// database. It needs TEST_DATABASE_URL (any database on a server the user may
// CREATE DATABASE on); tests skip when it is unset.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// New creates a throwaway database, applies every migrations/*.up.sql in order,
// and returns a pool on it. The database is dropped when the test ends.
func New(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()

	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect admin: %v", err)
	}
	defer func() { _ = admin.Close(ctx) }()

	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("random name: %v", err)
	}
	name := "famiy_test_" + hex.EncodeToString(buf)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create database: %v", err)
	}

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	cfg.ConnConfig.Database = name
	cfg.MaxConns = 20 // the concurrency test needs several simultaneous connections
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
		c, err := pgx.Connect(ctx, url)
		if err != nil {
			t.Logf("cleanup connect: %v", err)
			return
		}
		defer func() { _ = c.Close(ctx) }()
		if _, err := c.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)"); err != nil {
			t.Logf("drop %s: %v", name, err)
		}
	})

	migrate(t, pool)
	return pool
}

func migrate(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(file), "..", "..", "..", "migrations")
	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("find migrations in %s: %v", dir, err)
	}
	sort.Strings(files)
	for _, f := range files {
		sql, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		// No arguments => simple protocol, so a multi-statement file is allowed.
		if _, err := pool.Exec(context.Background(), string(sql)); err != nil {
			t.Fatalf("apply %s: %v", filepath.Base(f), err)
		}
	}
}
