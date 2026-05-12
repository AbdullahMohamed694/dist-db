package health

import (
	"log"
	"net/http"
	"time"

	"dist-db/master/db"
)

var httpClient = &http.Client{
	Timeout: 500 * time.Millisecond,   // fail fast if worker doesn't answer
}

func Start() {
	log.Println("Health checker started (immediate detection mode)")

	// Run the check immediately on startup, then every second
	checkWorkers()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		checkWorkers()
	}
}

func checkWorkers() {
	rows, err := db.DB.Query("SELECT id, address FROM distdb_system.worker_nodes")
	if err != nil {
		log.Printf("Health check query error: %v", err)
		return
	}
	defer rows.Close()

	type worker struct {
		id   string
		addr string
	}

	var workers []worker
	for rows.Next() {
		var id, addr string
		if err := rows.Scan(&id, &addr); err != nil {
			continue
		}
		workers = append(workers, worker{id, addr})
	}

	// Check all workers concurrently so one slow worker doesn't block others
	for _, w := range workers {
		go func(id, addr string) {
			resp, err := httpClient.Get(addr + "/health")
			if err != nil || resp.StatusCode != 200 {
				db.DB.Exec("UPDATE distdb_system.worker_nodes SET status = 'down' WHERE id = ?", id)
				log.Printf("Worker %s is DOWN", id)
			} else {
				db.DB.Exec("UPDATE distdb_system.worker_nodes SET status = 'up', last_heartbeat = NOW() WHERE id = ?", id)
				resp.Body.Close()
			}
		}(w.id, w.addr)
	}
}