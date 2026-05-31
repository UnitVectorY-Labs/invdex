package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/UnitVectorY-Labs/invdex/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps the PostgreSQL connection pool.
type DB struct {
	pool *pgxpool.Pool
}

// New creates a new database connection pool.
func New(ctx context.Context, databaseURL string) (*DB, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to database: %w", err)
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

// Migrate runs database migrations.
func (db *DB) Migrate(ctx context.Context) error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS tags (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name TEXT NOT NULL UNIQUE,
			category TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS items (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			image_url TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS item_tags (
			item_id UUID NOT NULL REFERENCES items(id) ON DELETE CASCADE,
			tag_id UUID NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
			PRIMARY KEY (item_id, tag_id)
		)`,
	}

	for _, m := range migrations {
		if _, err := db.pool.Exec(ctx, m); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	return nil
}

// CreateItem inserts a new item into the database.
func (db *DB) CreateItem(ctx context.Context, item *models.Item) error {
	err := db.pool.QueryRow(ctx,
		`INSERT INTO items (title, description, image_url) VALUES ($1, $2, $3) RETURNING id, created_at, updated_at`,
		item.Title, item.Description, item.ImageURL,
	).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create item: %w", err)
	}

	// Associate tags
	for _, tagName := range item.Tags {
		tagID, err := db.ensureTag(ctx, tagName)
		if err != nil {
			return err
		}
		if _, err := db.pool.Exec(ctx,
			`INSERT INTO item_tags (item_id, tag_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			item.ID, tagID,
		); err != nil {
			return fmt.Errorf("failed to associate tag: %w", err)
		}
	}

	return nil
}

// UpdateItem updates an existing item.
func (db *DB) UpdateItem(ctx context.Context, item *models.Item) error {
	_, err := db.pool.Exec(ctx,
		`UPDATE items SET title = $1, description = $2, image_url = $3, updated_at = NOW() WHERE id = $4`,
		item.Title, item.Description, item.ImageURL, item.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update item: %w", err)
	}

	// Remove existing tags and reassociate
	if _, err := db.pool.Exec(ctx, `DELETE FROM item_tags WHERE item_id = $1`, item.ID); err != nil {
		return fmt.Errorf("failed to clear item tags: %w", err)
	}

	for _, tagName := range item.Tags {
		tagID, err := db.ensureTag(ctx, tagName)
		if err != nil {
			return err
		}
		if _, err := db.pool.Exec(ctx,
			`INSERT INTO item_tags (item_id, tag_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			item.ID, tagID,
		); err != nil {
			return fmt.Errorf("failed to associate tag: %w", err)
		}
	}

	return nil
}

// DeleteItem removes an item from the database.
func (db *DB) DeleteItem(ctx context.Context, id string) error {
	_, err := db.pool.Exec(ctx, `DELETE FROM items WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete item: %w", err)
	}
	return nil
}

// GetItem retrieves a single item by ID.
func (db *DB) GetItem(ctx context.Context, id string) (*models.Item, error) {
	item := &models.Item{}
	err := db.pool.QueryRow(ctx,
		`SELECT id, title, description, image_url, created_at, updated_at FROM items WHERE id = $1`, id,
	).Scan(&item.ID, &item.Title, &item.Description, &item.ImageURL, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to get item: %w", err)
	}

	tags, err := db.getItemTags(ctx, item.ID)
	if err != nil {
		return nil, err
	}
	item.Tags = tags

	return item, nil
}

// ListItems returns all items, optionally filtered by tag.
func (db *DB) ListItems(ctx context.Context, tagFilter string) ([]*models.Item, error) {
	var query string
	var args []interface{}

	if tagFilter != "" {
		query = `SELECT DISTINCT i.id, i.title, i.description, i.image_url, i.created_at, i.updated_at
			FROM items i
			JOIN item_tags it ON i.id = it.item_id
			JOIN tags t ON it.tag_id = t.id
			WHERE t.name = $1
			ORDER BY i.updated_at DESC`
		args = append(args, tagFilter)
	} else {
		query = `SELECT id, title, description, image_url, created_at, updated_at FROM items ORDER BY updated_at DESC`
	}

	rows, err := db.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list items: %w", err)
	}
	defer rows.Close()

	var items []*models.Item
	for rows.Next() {
		item := &models.Item{}
		if err := rows.Scan(&item.ID, &item.Title, &item.Description, &item.ImageURL, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan item: %w", err)
		}
		tags, err := db.getItemTags(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		item.Tags = tags
		items = append(items, item)
	}

	return items, nil
}

// SearchItems searches items by title or description.
func (db *DB) SearchItems(ctx context.Context, query string) ([]*models.Item, error) {
	searchQuery := "%" + strings.ToLower(query) + "%"
	rows, err := db.pool.Query(ctx,
		`SELECT id, title, description, image_url, created_at, updated_at FROM items
		WHERE LOWER(title) LIKE $1 OR LOWER(description) LIKE $1
		ORDER BY updated_at DESC`, searchQuery,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to search items: %w", err)
	}
	defer rows.Close()

	var items []*models.Item
	for rows.Next() {
		item := &models.Item{}
		if err := rows.Scan(&item.ID, &item.Title, &item.Description, &item.ImageURL, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan item: %w", err)
		}
		tags, err := db.getItemTags(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		item.Tags = tags
		items = append(items, item)
	}

	return items, nil
}

// ListTags returns all tags.
func (db *DB) ListTags(ctx context.Context) ([]*models.Tag, error) {
	rows, err := db.pool.Query(ctx, `SELECT id, name, category FROM tags ORDER BY category, name`)
	if err != nil {
		return nil, fmt.Errorf("failed to list tags: %w", err)
	}
	defer rows.Close()

	var tags []*models.Tag
	for rows.Next() {
		tag := &models.Tag{}
		if err := rows.Scan(&tag.ID, &tag.Name, &tag.Category); err != nil {
			return nil, fmt.Errorf("failed to scan tag: %w", err)
		}
		tags = append(tags, tag)
	}

	return tags, nil
}

// CreateTag creates a new tag.
func (db *DB) CreateTag(ctx context.Context, tag *models.Tag) error {
	err := db.pool.QueryRow(ctx,
		`INSERT INTO tags (name, category) VALUES ($1, $2) RETURNING id`,
		tag.Name, tag.Category,
	).Scan(&tag.ID)
	if err != nil {
		return fmt.Errorf("failed to create tag: %w", err)
	}
	return nil
}

// DeleteTag removes a tag.
func (db *DB) DeleteTag(ctx context.Context, id string) error {
	_, err := db.pool.Exec(ctx, `DELETE FROM tags WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete tag: %w", err)
	}
	return nil
}

func (db *DB) ensureTag(ctx context.Context, name string) (string, error) {
	var id string
	err := db.pool.QueryRow(ctx,
		`INSERT INTO tags (name) VALUES ($1) ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name RETURNING id`,
		name,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("failed to ensure tag: %w", err)
	}
	return id, nil
}

func (db *DB) getItemTags(ctx context.Context, itemID string) ([]string, error) {
	rows, err := db.pool.Query(ctx,
		`SELECT t.name FROM tags t JOIN item_tags it ON t.id = it.tag_id WHERE it.item_id = $1 ORDER BY t.name`,
		itemID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get item tags: %w", err)
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan tag name: %w", err)
		}
		tags = append(tags, name)
	}

	return tags, nil
}
