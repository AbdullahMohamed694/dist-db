package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"dist-db/master/config"
)

var DB *sql.DB

func Init(cfg *config.Config) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/?parseTime=true",
		cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort)

	var err error
	DB, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	// Connection pool settings
	DB.SetMaxOpenConns(25)
	DB.SetMaxIdleConns(5)
	DB.SetConnMaxLifetime(5 * time.Minute)

	if err = DB.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	log.Println("Connected to MySQL server")

	// Create system database if not exists
	_, err = DB.Exec("CREATE DATABASE IF NOT EXISTS " + cfg.DBName)
	if err != nil {
		log.Fatalf("Failed to create system database: %v", err)
	}
	_, err = DB.Exec("USE " + cfg.DBName)
	if err != nil {
		log.Fatalf("Failed to select system database: %v", err)
	}

	// Create metadata tables
	createTables()

	// Clean up only successfully completed entries – keep pending/failed for retry
	_, err = DB.Exec("DELETE FROM distdb_system.replication_queue WHERE status = 'completed'")
	if err != nil {
		log.Printf("Warning: could not clean replication_queue: %v", err)
	} else {
		log.Println("Completed replication entries cleaned on startup")
	}
}

func createTables() {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS distdb_system.replication_queue (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			query TEXT NOT NULL,
			args JSON,
			source_worker_id VARCHAR(36) NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			retries INT DEFAULT 0,
			last_attempt TIMESTAMP NULL,
			status ENUM('pending','processing','completed','failed') DEFAULT 'pending'
		)`,
		`CREATE TABLE IF NOT EXISTS distdb_system.worker_nodes (
			id VARCHAR(36) PRIMARY KEY,
			name VARCHAR(100) NOT NULL,
			address VARCHAR(255) NOT NULL,
			status ENUM('up','down','unknown') DEFAULT 'unknown',
			last_heartbeat TIMESTAMP NULL,
			registered_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, q := range queries {
		if _, err := DB.Exec(q); err != nil {
			log.Fatalf("Failed to create tables: %v", err)
		}
	}
	log.Println("System tables ready")
}

// EnqueueReplication stores a SQL statement in the replication queue.
// This is used for writes that originate on the master itself (source is empty).
func EnqueueReplication(query string, args ...interface{}) {
	EnqueueReplicationWithSource(query, args, "")
}

// EnqueueReplicationWithSource inserts a write and records which worker sent it.
// If sourceWorkerID is empty, the write originated on the master.
func EnqueueReplicationWithSource(query string, args []interface{}, sourceWorkerID string) {
	argsJSON, _ := json.Marshal(args)
	_, err := DB.Exec("INSERT INTO distdb_system.replication_queue (query, args, source_worker_id, status) VALUES (?, ?, ?, 'pending')",
		query, string(argsJSON), sourceWorkerID)
	if err != nil {
		log.Printf("Failed to enqueue replication with source: %v", err)
	}
}