// Package graph provides Apache AGE integration for Zerelio's knowledge graph.
// It maintains a Cypher-queryable graph that mirrors the resources and
// relationships stored in the main PostgreSQL tables.
package graph

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

const graphName = "zerelio"

// Graph wraps a pgxpool and provides AGE-backed graph operations.
type Graph struct {
	pool   *pgxpool.Pool
	name   string
	logger *slog.Logger
}

// New creates a Graph using the given pool.
func New(pool *pgxpool.Pool, logger *slog.Logger) *Graph {
	return &Graph{pool: pool, name: graphName, logger: logger}
}

// Setup ensures the AGE extension is loaded and the graph exists.
// Safe to call multiple times (idempotent).
func (g *Graph) Setup(ctx context.Context) error {
	conn, err := g.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()

	// Load AGE extension (must exist in the postgres install).
	if _, err := conn.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS age;"); err != nil {
		return fmt.Errorf("create AGE extension: %w", err)
	}

	// Set search path so AGE catalog functions are visible.
	if _, err := conn.Exec(ctx, `SET search_path = ag_catalog, "$user", public;`); err != nil {
		return fmt.Errorf("set search_path: %w", err)
	}

	// Create the named graph (no-op if it already exists).
	_, err = conn.Exec(ctx,
		`SELECT * FROM ag_graph WHERE name = $1`,
		g.name,
	)

	var graphExists bool
	row := conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ag_graph WHERE name = $1)`, g.name)
	if err := row.Scan(&graphExists); err != nil {
		return fmt.Errorf("check graph existence: %w", err)
	}

	if !graphExists {
		if _, err := conn.Exec(ctx,
			`SELECT create_graph($1)`, g.name); err != nil {
			return fmt.Errorf("create_graph %s: %w", g.name, err)
		}
		g.logger.Info("AGE graph created", "name", g.name)
	} else {
		g.logger.Debug("AGE graph already exists", "name", g.name)
	}

	return nil
}
