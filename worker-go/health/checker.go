package health

import (
	"log"
	"net/http"
	"time"

	"dist-db/worker-go/db"
)

func Start() {
	log.Println("Health checker started (worker-master mode)")
	ticker := time.NewTicker(5 * time.Second)
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

	for rows.Next() {
		var id, addr string
		if err := rows.Scan(&id, &addr); err != nil {
			continue
		}
		resp, err := http.Get(addr + "/health")
		if err != nil || resp.StatusCode != 200 {
			db.DB.Exec("UPDATE distdb_system.worker_nodes SET status = 'down' WHERE id = ?", id)
			log.Printf("Worker %s is DOWN", id)
		} else {
			db.DB.Exec("UPDATE distdb_system.worker_nodes SET status = 'up', last_heartbeat = NOW() WHERE id = ?", id)
			resp.Body.Close()
		}
	}
}