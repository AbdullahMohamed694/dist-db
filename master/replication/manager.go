package replication

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"dist-db/master/db"
)

// Manager continuously polls the replication queue and sends to workers.
func Manager() {
	log.Println("Replication manager started")
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		processPending()
	}
}

func processPending() {
	rows, err := db.DB.Query(`
		SELECT id, query, args, source_worker_id FROM distdb_system.replication_queue
		WHERE status IN ('pending', 'failed')
		ORDER BY created_at ASC
		LIMIT 50
	`)
	if err != nil {
		log.Printf("Replication query error: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var query string
		var argsJSON *string
		var sourceWorkerID *string
		if err := rows.Scan(&id, &query, &argsJSON, &sourceWorkerID); err != nil {
			continue
		}

		var args []interface{}
		if argsJSON != nil && *argsJSON != "" {
			json.Unmarshal([]byte(*argsJSON), &args)
		}

		source := ""
		if sourceWorkerID != nil {
			source = *sourceWorkerID
		}

		db.DB.Exec("UPDATE distdb_system.replication_queue SET status = 'processing' WHERE id = ?", id)

		success := sendToWorkers(query, args, source)

		if success {
			db.DB.Exec("UPDATE distdb_system.replication_queue SET status = 'completed' WHERE id = ?", id)
		} else {
			db.DB.Exec(`UPDATE distdb_system.replication_queue
				SET status = 'failed', retries = retries + 1, last_attempt = NOW()
				WHERE id = ?`, id)
		}
	}
}

func sendToWorkers(query string, args []interface{}, sourceWorkerID string) bool {
	// Query ALL workers, not just 'up' ones – so we know if any missed the update
	workerRows, err := db.DB.Query("SELECT id, address, status FROM distdb_system.worker_nodes")
	if err != nil {
		return false
	}
	defer workerRows.Close()

	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		apiKey = "distdb-default-key"
	}

	body := map[string]interface{}{
		"query": query,
		"args":  args,
	}
	jsonBody, _ := json.Marshal(body)

	allOk := true
	for workerRows.Next() {
		var workerID, addr, status string
		if err := workerRows.Scan(&workerID, &addr, &status); err != nil {
			continue
		}

		// Skip the worker that sent this write to avoid loops
		if sourceWorkerID != "" && workerID == sourceWorkerID {
			log.Printf("Skipping source worker %s", workerID)
			continue
		}

		// If worker is down, we can't send to it → mark as failed
		if status != "up" {
			log.Printf("Worker %s is down, will retry later", workerID)
			allOk = false
			continue
		}

		// Send to healthy worker with API key
		req, _ := http.NewRequest("POST", addr+"/api/replicate", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", apiKey)

		resp, err := http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode >= 400 {
			log.Printf("Failed to replicate to %s: %v", addr, err)
			allOk = false
		}
		if resp != nil {
			resp.Body.Close()
		}
	}
	return allOk
}
