package api

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"dist-db/master/db"
)

// CreateDatabase creates a new user database.
// Request body: { "name": "mydb" }
func CreateDatabase(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	// Sanitise: only alphanumeric and underscore allowed
	req.Name = sanitizeDBName(req.Name)
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid database name"})
		return
	}

	// Execute DDL
	statement := "CREATE DATABASE IF NOT EXISTS `" + req.Name + "`"
	if _, err := db.DB.Exec(statement); err != nil {
		log.Printf("CreateDatabase error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create database"})
		return
	}

	// Record in replication queue
	db.EnqueueReplication(statement)

	log.Printf("Database %s created", req.Name)
	c.JSON(http.StatusOK, gin.H{"message": "database created", "name": req.Name})
}

// ListDatabases returns all databases except system ones.
func ListDatabases(c *gin.Context) {
	rows, err := db.DB.Query("SHOW DATABASES")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	databases := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		// Exclude system databases
		if name == "information_schema" || name == "mysql" || name == "performance_schema" || name == "sys" || name == "distdb_system" {
			continue
		}
		databases = append(databases, name)
	}
	c.JSON(http.StatusOK, gin.H{"databases": databases})
}

// DropDatabase drops a database (master only).
func DropDatabase(c *gin.Context) {
	dbName := c.Param("dbname")
	dbName = sanitizeDBName(dbName)

	if dbName == "" || dbName == "distdb_system" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or protected database"})
		return
	}

	statement := "DROP DATABASE IF EXISTS `" + dbName + "`"
	if _, err := db.DB.Exec(statement); err != nil {
		log.Printf("DropDatabase error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not drop database"})
		return
	}

	// Record replication
	db.EnqueueReplication(statement)

	log.Printf("Database %s dropped", dbName)
	c.JSON(http.StatusOK, gin.H{"message": "database dropped", "name": dbName})
}

// sanitizeDBName returns only safe characters: letters, digits, underscores.
func sanitizeDBName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
}