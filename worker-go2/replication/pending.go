package replication

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"dist-db/worker-go/config"
	"dist-db/worker-go/db"
)

func StartPendingReplayer() {
	log.Println("Pending writes replayer started")
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		retryPending()
	}
}

func retryPending() {
	query := "SELECT id, query, args FROM " + db.WorkerSystemDB + ".pending_writes ORDER BY id LIMIT 50"
	rows, err := db.DB.Query(query)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var q string
		var argsJSON string
		if err := rows.Scan(&id, &q, &argsJSON); err != nil {
			continue
		}
		var args []interface{}
		json.Unmarshal([]byte(argsJSON), &args)

		body := map[string]interface{}{"query": q, "args": args}
		jsonBody, _ := json.Marshal(body)
		cfg := config.Load()
		resp, err := http.Post(cfg.MasterURL+"/api/replicate", "application/json", bytes.NewBuffer(jsonBody))
		if err == nil && resp.StatusCode < 400 {
			db.DB.Exec("DELETE FROM "+db.WorkerSystemDB+".pending_writes WHERE id = ?", id)
		}
		if resp != nil {
			resp.Body.Close()
		}
	}
}