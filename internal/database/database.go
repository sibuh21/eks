package database

import (
	"context"
	"fmt"
	"time"

	"github.com/sibuh/eks-echo-app/internal/model"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a PostgreSQL connection pool.
type DB struct {
	pool *pgxpool.Pool
}

// New creates a new database connection pool.
func New(databaseURL string) (*DB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("unable to ping database: %w", err)
	}

	return &DB{pool: pool}, nil
}

// Close closes the database connection pool.
func (db *DB) Close() {
	db.pool.Close()
}

// Migrate creates the required tables if they don't exist.
func (db *DB) Migrate() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	query := `
		CREATE TABLE IF NOT EXISTS items (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			description TEXT DEFAULT '',
			price NUMERIC(10,2) NOT NULL CHECK (price > 0),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_items_name ON items(name);
	`

	_, err := db.pool.Exec(ctx, query)
	return err
}

// Ping checks the database connection.
func (db *DB) Ping(ctx context.Context) error {
	return db.pool.Ping(ctx)
}

// CreateItem inserts a new item into the database.
func (db *DB) CreateItem(ctx context.Context, item *model.Item) error {
	query := `
		INSERT INTO items (name, description, price)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at
	`
	return db.pool.QueryRow(ctx, query, item.Name, item.Description, item.Price).
		Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt)
}

// GetItem retrieves an item by its ID.
func (db *DB) GetItem(ctx context.Context, id int) (*model.Item, error) {
	item := &model.Item{}
	query := `SELECT id, name, description, price, created_at, updated_at FROM items WHERE id = $1`
	err := db.pool.QueryRow(ctx, query, id).
		Scan(&item.ID, &item.Name, &item.Description, &item.Price, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return item, nil
}

// ListItems retrieves all items.
func (db *DB) ListItems(ctx context.Context) ([]model.Item, error) {
	query := `SELECT id, name, description, price, created_at, updated_at FROM items ORDER BY id DESC`
	rows, err := db.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.Item
	for rows.Next() {
		var item model.Item
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.Price, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// UpdateItem updates an existing item.
func (db *DB) UpdateItem(ctx context.Context, id int, item *model.Item) error {
	query := `
		UPDATE items
		SET name = $2, description = $3, price = $4, updated_at = NOW()
		WHERE id = $1
		RETURNING updated_at
	`
	err := db.pool.QueryRow(ctx, query, id, item.Name, item.Description, item.Price).
		Scan(&item.UpdatedAt)
	item.ID = id
	return err
}

// DeleteItem removes an item by its ID.
func (db *DB) DeleteItem(ctx context.Context, id int) error {
	query := `DELETE FROM items WHERE id = $1`
	ct, err := db.pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("item not found")
	}
	return nil
}
