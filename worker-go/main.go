package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"dist-db/worker-go/config"
	"dist-db/worker-go/db"
	"dist-db/shared"
	"dist-db/worker-go/api"
)

func main() {
	cfg := config.Load()

	// Check if this node was previously promoted to master
	if b, err := os.ReadFile("master_status"); err == nil && len(b) > 0 && b[0] == '1' {
		cfg.IsMaster = true
	}

	cfg.WorkerID = uuid.New().String()

	// Initialize local MySQL
	db.Init(cfg)

	// Register with the current master
	registerWithMaster(cfg, cfg.WorkerID)

	router := gin.Default()

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "node": cfg.WorkerName})
	})

	// Replication endpoint – used by master to push SQL
	router.POST("/api/replicate", replicateHandler)

	// Worker CRUD
	router.POST("/api/databases/:dbname/tables/:table/rows", api.InsertRow)
	router.GET("/api/databases/:dbname/tables/:table/rows", api.SelectRows)
	router.PUT("/api/databases/:dbname/tables/:table/rows/:id", api.UpdateRow)
	router.DELETE("/api/databases/:dbname/tables/:table/rows/:id", api.DeleteRow)

	// Master-only routes (return 503 if not master)
	router.POST("/api/databases", api.CreateDatabase)
	router.GET("/api/databases", api.ListDatabases)
	router.DELETE("/api/databases/:dbname", api.DropDatabase)
	router.POST("/api/databases/:dbname/tables", api.CreateTable)
	router.GET("/api/databases/:dbname/tables", api.ListTables)
	router.DELETE("/api/databases/:dbname/tables/:table", api.DropTable)

	// Node management
	router.POST("/api/node/promote", api.PromoteNode)
	router.POST("/api/node/update-master", api.UpdateMaster)

	// Start pending writes replayer
	//go replication.StartPendingReplayer()

	log.Printf("%s listening on :%s", cfg.WorkerName, cfg.ServerPort)
	if err := router.Run(":" + cfg.ServerPort); err != nil {
		log.Fatalf("Failed to start worker: %v", err)
	}
}

func replicateHandler(c *gin.Context) {
	var req struct {
		Query string        `json:"query" binding:"required"`
		Args  []interface{} `json:"args"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	_, err := db.DB.Exec(req.Query, req.Args...)
	if err != nil {
		log.Printf("Replication error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Println("Replication executed successfully")
	c.JSON(http.StatusOK, gin.H{"message": "replicated"})
}

func registerWithMaster(cfg *config.Config, workerID string) {
	reqBody := shared.WorkerRegisterRequest{
		ID:      workerID,
		Name:    cfg.WorkerName,
		Address: "http://" + getLocalIP() + ":" + cfg.ServerPort,
	}

	jsonData, _ := json.Marshal(reqBody)
	resp, err := http.Post(cfg.MasterURL+"/api/workers/register",
		"application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Warning: could not register with master: %v", err)
		return
	}
	defer resp.Body.Close()

	var result shared.WorkerRegisterResponse
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Success {
		log.Println("Registered with master successfully")
	} else {
		log.Printf("Registration failed: %s", result.Message)
	}
}

func getLocalIP() string {
	// Use localhost for development; replace with real IP detection later
	return "localhost"
}