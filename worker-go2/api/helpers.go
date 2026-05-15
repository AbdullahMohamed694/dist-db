package api

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"dist-db/worker-go/config"
	"dist-db/worker-go/db"
	"strings"
	"os"
)


func ForwardToMaster(query string, args []interface{}) {
	go func() {
		apiKey := os.Getenv("API_KEY")
		if apiKey == "" {
			apiKey = "distdb-default-key"
		}

		body := map[string]interface{}{
			"query": query,
			"args":  args,
		}
		jsonBytes, _ := json.Marshal(body)

		cfg := config.Load()
		req, _ := http.NewRequest("POST", cfg.MasterURL+"/api/replicate", bytes.NewBuffer(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", apiKey)
		req.Header.Set("X-Source-Worker-ID", cfg.WorkerID)

		resp, err := http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode >= 400 {
			log.Printf("Master unreachable, queuing write locally")
			db.EnqueuePending(query, args)
		}
		if resp != nil {
			resp.Body.Close()
		}
	}()
}
func sanitizeDBName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func sanitizeIdentifier(s string) string {
	return sanitizeDBName(s)
}