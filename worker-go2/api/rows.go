package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"dist-db/worker-go/config"
	"dist-db/worker-go/db"
)

// ---------- CRUD Handlers ----------
func InsertRow(c *gin.Context) {
	dbName := sanitizeDBName(c.Param("dbname"))
	tableName := sanitizeIdentifier(c.Param("table"))
	if dbName == "" || tableName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid db/table"})
		return
	}

	var cols map[string]interface{}
	if err := c.ShouldBindJSON(&cols); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
		return
	}

	id := uuid.New().String()
	cols["id"] = id

	colNames, placeholders, values := []string{}, []string{}, []interface{}{}
	for col, val := range cols {
		col = sanitizeIdentifier(col)
		if col == "" {
			continue
		}
		colNames = append(colNames, "`"+col+"`")
		placeholders = append(placeholders, "?")
		values = append(values, val)
	}

	query := fmt.Sprintf("INSERT INTO `%s`.`%s` (%s) VALUES (%s)",
		dbName, tableName, strings.Join(colNames, ", "), strings.Join(placeholders, ", "))

	_, err := db.DB.Exec(query, values...)
	if err != nil {
		log.Printf("Worker Insert error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	cfg := config.Load()
	forwardToMaster(query, values, cfg.WorkerID)

	c.JSON(http.StatusCreated, gin.H{"message": "row inserted", "id": id})
}

func SelectRows(c *gin.Context) {
	dbName := sanitizeDBName(c.Param("dbname"))
	tableName := sanitizeIdentifier(c.Param("table"))
	if dbName == "" || tableName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid db/table"})
		return
	}

	whereClause := c.Query("where")
	query := fmt.Sprintf("SELECT * FROM `%s`.`%s`", dbName, tableName)
	if whereClause != "" {
		query += " WHERE " + whereClause
	}

	rows, err := db.QueryRowMap(query)
	if err != nil {
		log.Printf("Worker Select error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rows": rows})
}

func UpdateRow(c *gin.Context) {
	dbName := sanitizeDBName(c.Param("dbname"))
	tableName := sanitizeIdentifier(c.Param("table"))
	id := c.Param("id")
	if dbName == "" || tableName == "" || id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid parameters"})
		return
	}

	var cols map[string]interface{}
	if err := c.ShouldBindJSON(&cols); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
		return
	}

	setClauses, values := []string{}, []interface{}{}
	for col, val := range cols {
		col = sanitizeIdentifier(col)
		if col == "" {
			continue
		}
		setClauses = append(setClauses, fmt.Sprintf("`%s` = ?", col))
		values = append(values, val)
	}
	if len(setClauses) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no columns"})
		return
	}

	query := fmt.Sprintf("UPDATE `%s`.`%s` SET %s WHERE `id` = ?",
		dbName, tableName, strings.Join(setClauses, ", "))
	values = append(values, id)

	_, err := db.DB.Exec(query, values...)
	if err != nil {
		log.Printf("Worker Update error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	cfg := config.Load()
	forwardToMaster(query, values, cfg.WorkerID)
	c.JSON(http.StatusOK, gin.H{"message": "row updated"})
}

func DeleteRow(c *gin.Context) {
	dbName := sanitizeDBName(c.Param("dbname"))
	tableName := sanitizeIdentifier(c.Param("table"))
	id := c.Param("id")
	if dbName == "" || tableName == "" || id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid parameters"})
		return
	}

	query := "DELETE FROM `" + dbName + "`.`" + tableName + "` WHERE `id` = ?"
	_, err := db.DB.Exec(query, id)
	if err != nil {
		log.Printf("Worker Delete error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	cfg := config.Load()
	forwardToMaster(query, []interface{}{id}, cfg.WorkerID)
	c.JSON(http.StatusOK, gin.H{"message": "row deleted"})
}

// ---------- Master forwarding ----------
func forwardToMaster(query string, args []interface{}, workerID string) {
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
		req.Header.Set("X-Source-Worker-ID", workerID)
		req.Header.Set("X-API-Key", apiKey)   // <-- ADDED: master requires this

		resp, err := http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode >= 400 {
			log.Printf("Master unreachable, queuing locally")
			db.EnqueuePending(query, args)
		}
		if resp != nil {
			resp.Body.Close()
		}
	}()
}