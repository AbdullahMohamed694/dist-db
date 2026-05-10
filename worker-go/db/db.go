package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"dist-db/worker-go/config"
)

var DB *sql.DB
var WorkerSystemDB string   // set in Init

func Init(cfg *config.Config) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/?parseTime=true",
		cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort)

	var err error
	DB, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	DB.SetMaxOpenConns(25)
	DB.SetMaxIdleConns(5)
	DB.SetConnMaxLifetime(5 * time.Minute)

	if err = DB.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	log.Println("Worker-Go connected to MySQL")

	// Create system database if not exists
	WorkerSystemDB = cfg.DBName
	_, err = DB.Exec("CREATE DATABASE IF NOT EXISTS " + WorkerSystemDB)
	if err != nil {
		log.Fatalf("Failed to create system database: %v", err)
	}

	// Create pending_writes table inside the system database
	_, err = DB.Exec(`CREATE TABLE IF NOT EXISTS ` + WorkerSystemDB + `.pending_writes (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		query TEXT NOT NULL,
		args JSON,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		log.Fatalf("Failed to create pending_writes: %v", err)
	}
	log.Println("Pending writes table ready")
}

// EnqueuePending inserts a query into the local pending queue (used when master is down)
func EnqueuePending(query string, args []interface{}) {
	argsJSON, _ := json.Marshal(args)
	_, err := DB.Exec("INSERT INTO "+WorkerSystemDB+".pending_writes (query, args) VALUES (?, ?)", query, string(argsJSON))
	if err != nil {
		log.Printf("Failed to enqueue pending write: %v", err)
	}
}

func InitMasterSystem() error {
	_, err := DB.Exec("CREATE DATABASE IF NOT EXISTS distdb_system")
	if err != nil {
		return err
	}
	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS distdb_system.replication_queue (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			query TEXT NOT NULL,
			args JSON,
			source_worker_id VARCHAR(36) NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			retries INT DEFAULT 0,
			last_attempt TIMESTAMP NULL,
			status ENUM('pending','processing','completed','failed') DEFAULT 'pending'
		)
	`)
	if err != nil {
		return err
	}
	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS distdb_system.worker_nodes (
			id VARCHAR(36) PRIMARY KEY,
			name VARCHAR(100) NOT NULL,
			address VARCHAR(255) NOT NULL,
			status ENUM('up','down','unknown') DEFAULT 'unknown',
			last_heartbeat TIMESTAMP NULL,
			registered_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}
func EnqueueReplication(query string, args ...interface{}) {
	argsJSON, _ := json.Marshal(args)
	_, err := DB.Exec("INSERT INTO distdb_system.replication_queue (query, args, status) VALUES (?, ?, 'pending')", query, string(argsJSON))
	if err != nil {
		log.Printf("Failed to enqueue replication: %v", err)
	}
}