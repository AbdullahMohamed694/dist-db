package health

import (
	"log"
	"net/http"
	"os"
	"time"

	"dist-db/worker-go/config"
	"dist-db/worker-go/db"
)

var httpClient = &http.Client{Timeout: 2 * time.Second}

// StartAutoFailover continuously checks if the master is alive.
// If the master is down for longer than failoverAfter, this worker promotes itself.
func StartAutoFailover() {
	cfg := config.Cfg
	if !cfg.Standby {
		log.Println("Auto‑failover disabled (STANDBY not set)")
		return
	}

	failoverAfter := 15 * time.Second
	checkInterval := 3 * time.Second
	masterDownSince := time.Time{}
	originalMasterURL := cfg.MasterURL // save the original master URL
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		apiKey = "distdb-default-key"
	}

	log.Printf("Auto‑failover enabled. Master: %s", originalMasterURL)

	for {
		time.Sleep(checkInterval)

		req, _ := http.NewRequest("GET", originalMasterURL+"/health", nil)
		req.Header.Set("X-API-Key", apiKey)
		resp, err := httpClient.Do(req)

		if err != nil || resp.StatusCode != 200 {
			// Master is DOWN
			if masterDownSince.IsZero() {
				masterDownSince = time.Now()
				log.Println("Master appears DOWN, starting failover timer")
			}

			if time.Since(masterDownSince) >= failoverAfter && !cfg.IsMaster {
				log.Println("Failover timer expired – promoting self to master")
				promoteSelf()
			}
		} else {
			// Master is UP
			masterDownSince = time.Time{}
			if resp != nil {
				resp.Body.Close()
			}

			// If this node was the temporary master and the original master is back, DEMOTE self
			if cfg.IsMaster && originalMasterURL != "http://"+cfg.WorkerIP+":"+cfg.ServerPort {
				log.Println("Original master is back – demoting self")
				demoteSelf(originalMasterURL)
			}
		}
	}
}

func demoteSelf(originalMasterURL string) {
	cfg := config.Cfg
	cfg.IsMaster = false
	cfg.MasterURL = originalMasterURL
	os.Remove("master_status")

	log.Println("Self‑demotion complete – back to worker mode")
}

func promoteSelf() {
    log.Println("Self‑promoting to master…")

    if err := db.InitMasterSystem(); err != nil {
        log.Printf("Failed to init master system: %v", err)
        return
    }

    // Update the global config
    config.Cfg.IsMaster = true
    os.WriteFile("master_status", []byte("1"), 0644)

    log.Println("Self‑promotion complete – this node is now the master")
}