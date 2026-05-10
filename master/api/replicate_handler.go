package api

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"dist-db/master/db"
)

// ReplicateFromWorker receives a write forwarded by a worker.
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
	if sourceWorkerID == "" {
		log.Println("Warning: no source worker ID in replicate request")
	}

	// Enqueue the write with source information
	db.EnqueueReplicationWithSource(req.Query, req.Args, sourceWorkerID)

	c.JSON(http.StatusOK, gin.H{"message": "queued for replication"})
}