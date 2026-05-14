package api

import (
	"bytes"
	"encoding/json"      // ← this was missing
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"dist-db/worker-go/config"
	"dist-db/worker-go/db"
	"dist-db/worker-go/health"
	"dist-db/worker-go/replication"
	"dist-db/shared"      // ← this was missing
)

func PromoteNode(c *gin.Context) {
	if config.Cfg.IsMaster {
		c.JSON(http.StatusOK, gin.H{"message": "already master"})
		return
	}

	// Initialize master system tables locally
	if err := db.InitMasterSystem(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to init system db: " + err.Error()})
		return
	}

	config.Cfg.IsMaster = true
	os.WriteFile("master_status", []byte("1"), 0644)

	// Start background master services
	go health.Start()
	go replication.Manager()

	log.Println("Promoted to master")
	c.JSON(http.StatusOK, gin.H{"message": "promoted to master"})
}

func UpdateMaster(c *gin.Context) {
	var req struct {
		MasterURL string `json:"master_url" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "master_url required"})
		return
	}

	config.Cfg.MasterURL = req.MasterURL
	// Re‑register with new master
	RegisterWorkerWithURL(config.Cfg.WorkerID, config.Cfg.WorkerName, "http://"+config.Cfg.WorkerIP+":"+config.Cfg.ServerPort, req.MasterURL)

	c.JSON(http.StatusOK, gin.H{"message": "master updated"})
}

func RegisterWorkerWithURL(id, name, address, masterURL string) {
	reqBody := shared.WorkerRegisterRequest{
		ID: id, Name: name, Address: address,
	}
	jsonData, _ := json.Marshal(reqBody)
	http.Post(masterURL+"/api/workers/register", "application/json", bytes.NewBuffer(jsonData))
}