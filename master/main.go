package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"dist-db/master/api"
	"dist-db/master/config"
	"dist-db/master/db"
	"dist-db/master/health"
	"dist-db/master/replication"
)

func main() {
	cfg := config.Load()
	db.Init(cfg)

	router := gin.Default()

	// Health checks
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "node": "master"})
	})
	router.GET("/db-health", func(c *gin.Context) {
		if err := db.DB.Ping(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "db connected", "dbname": cfg.DBName})
	})

	// Worker routes (only ONE registration endpoint)
	router.GET("/api/workers", api.ListWorkers)
	router.POST("/api/workers/register", api.RegisterWorker)

	router.POST("/api/replicate", api.ReplicateFromWorker)

	// Database routes
	router.POST("/api/databases", api.CreateDatabase)
	router.GET("/api/databases", api.ListDatabases)
	router.DELETE("/api/databases/:dbname", api.DropDatabase)

	// Table routes
	router.POST("/api/databases/:dbname/tables", api.CreateTable)
	router.GET("/api/databases/:dbname/tables", api.ListTables)

	router.GET("/api/databases/:dbname/tables/:table/schema", api.GetTableSchema)

	// Row CRUD routes
	router.POST("/api/databases/:dbname/tables/:table/rows", api.InsertRow)
	router.GET("/api/databases/:dbname/tables/:table/rows", api.SelectRows)
	router.PUT("/api/databases/:dbname/tables/:table/rows/:id", api.UpdateRow)
	router.DELETE("/api/databases/:dbname/tables/:table/rows/:id", api.DeleteRow)

	router.DELETE("/api/databases/:dbname/tables/:table", api.DropTable)

	// Serve the dashboard
	router.StaticFile("/dashboard", "../frontend/dashboard.html")

	// Start background services
	go health.Start()
	go replication.Manager()

	log.Printf("Master node starting on :%s", cfg.ServerPort)
	if err := router.Run(":" + cfg.ServerPort); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}