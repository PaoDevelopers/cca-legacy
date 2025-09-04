/*
 * Database handling
 *
 * Copyright (C) 2024  Runxi Yu <https://runxiyu.org>
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */

package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var db *pgxpool.Pool

const pgErrUniqueViolation = "23505"

/*
 * This must be run during setup, before the database is accessed by any
 * means. Otherwise, db would be a null pointer.
 */
func setupDatabase() error {
	var err error
	if config.DB.Type != "postgres" {
		return errors.New("only postgres databases are supported")
	}

	// Parse the connection string into a Config object
	poolConfig, err := pgxpool.ParseConfig(config.DB.Conn)
	if err != nil {
		return fmt.Errorf("parse database config: %w", err)
	}

	// Configure the connection pool for high concurrency
	poolConfig.MaxConns = 500                       // Maximum number of connections in the pool
	poolConfig.MinConns = 10                        // Minimum idle connections to maintain
	poolConfig.MaxConnLifetime = 5 * time.Hour      // Maximum lifetime of a connection
	poolConfig.MaxConnIdleTime = 30 * time.Minute   // Maximum idle time before recycling
	poolConfig.HealthCheckPeriod = 40 * time.Second // How often to check connection health

	// Connection acquisition timeout and cancellation
	poolConfig.ConnConfig.ConnectTimeout = 5 * time.Second

	// Create context with timeout for initial connection
	// Create the connection pool with our optimized settings
	db, err = pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	// Verify the connection works
	if err := db.Ping(context.Background()); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	return nil
}
