package api

import (
	"log"
	"net/http"
	"fmt"
	"github.com/gin-gonic/gin"
	"dist-db/worker-go/config"
	"dist-db/worker-go/db"
	"database/sql"
	"time"
)

func requireMaster(c *gin.Context) bool {
	if !config.Cfg.IsMaster {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "not master"})
		return false
	}
	return true
}

func CreateDatabase(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name required"})
		return
	}
	query := "CREATE DATABASE IF NOT EXISTS `" + sanitizeDBName(req.Name) + "`"
	_, err := db.DB.Exec(query)
	if err != nil {
		log.Printf("CreateDatabase error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Enqueue replication (will be processed by this node's manager if master)
	ForwardToMaster(query, []interface{}{})
	c.JSON(http.StatusOK, gin.H{"message": "database created", "name": req.Name})
}

func ListDatabases(c *gin.Context) {
	// (no master guard – listing is safe for all nodes)
	rows, err := db.DB.Query("SHOW DATABASES")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	var databases []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			if name != "information_schema" && name != "mysql" && name != "performance_schema" && name != "sys" && name != "distdb_system" && name != "distdb_worker" {
				databases = append(databases, name)
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"databases": databases})
}

func DropDatabase(c *gin.Context) {
	if !requireMaster(c) { return }
	dbName := sanitizeDBName(c.Param("dbname"))
	if dbName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid db name"})
		return
	}
	query := "DROP DATABASE IF EXISTS `" + dbName + "`"
	_, err := db.DB.Exec(query)
	if err != nil {
		log.Printf("DropDatabase error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ForwardToMaster(query, []interface{}{})
	c.JSON(http.StatusOK, gin.H{"message": "database dropped"})
}

func CreateTable(c *gin.Context) {
	dbName := sanitizeDBName(c.Param("dbname"))
	var req struct {
		Name    string                   `json:"name" binding:"required"`
		Columns []map[string]interface{} `json:"columns"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	tableName := sanitizeIdentifier(req.Name)
	// build column definitions with an 'id' primary key
	cols := "`id` CHAR(36) PRIMARY KEY"
	for _, col := range req.Columns {
		colName, _ := col["name"].(string)
		colType, _ := col["type"].(string)
		if colName != "" && colType != "" {
			cols += ", `" + sanitizeIdentifier(colName) + "` " + colType
		}
	}
	query := "CREATE TABLE IF NOT EXISTS `" + dbName + "`.`" + tableName + "` (" + cols + ")"
	_, err := db.DB.Exec(query)
	if err != nil {
		log.Printf("CreateTable error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ForwardToMaster(query, []interface{}{})
	c.JSON(http.StatusOK, gin.H{"message": "table created"})
}

func ListTables(c *gin.Context) {
	dbName := sanitizeDBName(c.Param("dbname"))
	rows, err := db.DB.Query("SHOW TABLES FROM `" + dbName + "`")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			tables = append(tables, name)
		}
	}
	c.JSON(http.StatusOK, gin.H{"database": dbName, "tables": tables})
}

func DropTable(c *gin.Context) {
	dbName := sanitizeDBName(c.Param("dbname"))
	tableName := sanitizeIdentifier(c.Param("table"))
	if dbName == "" || tableName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid parameters"})
		return
	}
	query := "DROP TABLE IF EXISTS `" + dbName + "`.`" + tableName + "`"
	_, err := db.DB.Exec(query)
	if err != nil {
		log.Printf("DropTable error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ForwardToMaster(query, []interface{}{})
	c.JSON(http.StatusOK, gin.H{"message": "table dropped"})
}
func GetTableSchema(c *gin.Context) {
	dbName := sanitizeDBName(c.Param("dbname"))
	tableName := sanitizeIdentifier(c.Param("table"))
	if dbName == "" || tableName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid database or table"})
		return
	}

	rows, err := db.DB.Query("SHOW COLUMNS FROM `" + dbName + "`.`" + tableName + "`")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	columns := []gin.H{}
	for rows.Next() {
		var field, colType, null, key, extra interface{}
		var def interface{}
		if err := rows.Scan(&field, &colType, &null, &key, &def, &extra); err != nil {
			continue
		}
		// Convert []byte to string
		name := fmt.Sprintf("%v", field)
		if b, ok := field.([]byte); ok {
			name = string(b)
		}
		typeStr := fmt.Sprintf("%v", colType)
		if b, ok := colType.([]byte); ok {
			typeStr = string(b)
		}
		columns = append(columns, gin.H{
			"name": name,
			"type": typeStr,
		})
	}
	c.JSON(http.StatusOK, gin.H{"columns": columns})
}

func ListWorkers(c *gin.Context) {
	if !requireMaster(c) { return }

	rows, err := db.DB.Query("SELECT id, name, address, status, last_heartbeat FROM distdb_system.worker_nodes ORDER BY last_heartbeat DESC")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	seen := map[string]bool{}
	workers := []gin.H{}
	for rows.Next() {
		var id, name, address, status string
		var lastHb sql.NullTime
		if err := rows.Scan(&id, &name, &address, &status, &lastHb); err != nil {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		worker := gin.H{
			"id":      id,
			"name":    name,
			"address": address,
			"status":  status,
		}
		if lastHb.Valid {
			worker["last_heartbeat"] = lastHb.Time.Format(time.RFC3339)
		}
		workers = append(workers, worker)
	}
	c.JSON(http.StatusOK, gin.H{"workers": workers})
}