package api

import (
	"log"
	"net/http"
	"time"
	"database/sql"
	"github.com/gin-gonic/gin"
	"dist-db/master/db"
	"dist-db/shared"
)

// RegisterWorker handles POST /api/workers/register
func RegisterWorker(c *gin.Context) {
	var req shared.WorkerRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, shared.WorkerRegisterResponse{
			Success: false,
			Message: "Invalid request body",
		})
		return
	}

	if req.ID == "" || req.Name == "" || req.Address == "" {
		c.JSON(http.StatusBadRequest, shared.WorkerRegisterResponse{
			Success: false,
			Message: "id, name, and address are required",
		})
		return
	}

	// Insert or update the worker in the database
	_, err := db.DB.Exec(
    `INSERT INTO distdb_system.worker_nodes (id, name, address, status, last_heartbeat)
     VALUES (?, ?, ?, 'up', NOW())
     ON DUPLICATE KEY UPDATE
     name = VALUES(name), address = VALUES(address),
     status = 'up', last_heartbeat = NOW()`,
    req.ID, req.Name, req.Address,
		)
	if err != nil {
		log.Printf("Failed to register worker %s: %v", req.Name, err)
		c.JSON(http.StatusInternalServerError, shared.WorkerRegisterResponse{
			Success: false,
			Message: "Database error",
		})
		return
	}

	log.Printf("Worker registered: %s (%s) at %s", req.Name, req.ID, req.Address)
	c.JSON(http.StatusOK, shared.WorkerRegisterResponse{
		Success: true,
		Message: "Worker registered successfully",
	})
	
}

// ListWorkers returns all registered workers and their status
// ListWorkers returns one row per unique worker name (latest status/ heartbeat)
func ListWorkers(c *gin.Context) {
	rows, err := db.DB.Query(`
		SELECT id, name, address, status, last_heartbeat
		FROM distdb_system.worker_nodes
		ORDER BY last_heartbeat DESC
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	seen := map[string]bool{}
	workers := []gin.H{}

	for rows.Next() {
		var id, name, address, status string
		var lastHb sql.NullTime
		if err := rows.Scan(&id, &name, &address, &status, &lastHb); err != nil {
			continue
		}
		if seen[name] {
			continue   // skip older duplicates
		}
		seen[name] = true

		worker := gin.H{
			"id":      id,
			"name":    name,
			"address": address,
			"status":  status,
		}
		if lastHb.Valid {
			worker["last_heartbeat"] = lastHb.Time.Format(time.RFC3339)
		}
		workers = append(workers, worker)
	}

	c.JSON(http.StatusOK, gin.H{"workers": workers})
}