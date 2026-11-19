package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var files embed.FS

func Up(ctx context.Context, databaseURL string) error {
	db, err := open(databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := prepare(); err != nil {
		return err
	}

	if err := goose.UpContext(ctx, db, "."); err != nil {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

func Down(ctx context.Context, databaseURL string) error {
	db, err := open(databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := prepare(); err != nil {
		return err
	}

	if err := goose.DownContext(ctx, db, "."); err != nil {
		return fmt.Errorf("migrate down: %w", err)
	}
	return nil
}

func Status(ctx context.Context, databaseURL string) error {
	db, err := open(databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := prepare(); err != nil {
		return err
	}

	if err := goose.StatusContext(ctx, db, "."); err != nil {
		return fmt.Errorf("migrate status: %w", err)
	}
	return nil
}

func open(databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open migration database: %w", err)
	}
	return db, nil
}

func prepare() error {
	goose.SetBaseFS(files)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	return nil
}
