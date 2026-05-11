package api

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"dist-db/master/db"
)

func ReplicateFromWorker(c *gin.Context) {
	var req struct {
		Query string        `json:"query" binding:"required"`
		Args  []interface{} `json:"args"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	sourceWorkerID := c.GetHeader("X-Source-Worker-ID")

	// 1. Execute the statement on the master’s own database
	if _, err := db.DB.Exec(req.Query, req.Args...); err != nil {
		log.Printf("Failed to execute worker statement on master: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 2. Enqueue it so it gets replicated to other workers (the source worker will be skipped)
	db.EnqueueReplicationWithSource(req.Query, req.Args, sourceWorkerID)

	log.Printf("Executed and queued replication from worker %s: %s", sourceWorkerID, req.Query)
	c.JSON(http.StatusOK, gin.H{"message": "queued for replication"})
}