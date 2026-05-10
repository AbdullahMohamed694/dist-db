package api

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"dist-db/master/db"
)

// CreateTableRequest defines the columns for a new table.
type CreateTableRequest struct {
	Name    string                   `json:"name" binding:"required"`
	Columns []map[string]interface{} `json:"columns" binding:"required"` // [{"name": "col1", "type": "VARCHAR(100)"}, ...]
}

// CreateTable handles table creation.
func CreateTable(c *gin.Context) {
	dbName := sanitizeDBName(c.Param("dbname"))
	if dbName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid database name"})
		return
	}

	var req CreateTableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "table name and columns required"})
		return
	}
	tableName := sanitizeIdentifier(req.Name)
	if tableName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid table name"})
		return
	}

	// Build column definitions – force an 'id' column as primary key
	cols := []string{"`id` CHAR(36) PRIMARY KEY"}
	for _, col := range req.Columns {
		colName, ok1 := col["name"].(string)
		colType, ok2 := col["type"].(string)
		if ok1 && ok2 {
			colName = sanitizeIdentifier(colName)
			colType = strings.ToUpper(colType) // limit to simple types in production, we trust for learning
			if colName != "id" {
				cols = append(cols, fmt.Sprintf("`%s` %s", colName, colType))
			}
		}
	}

	statement := fmt.Sprintf("CREATE TABLE IF NOT EXISTS `%s`.`%s` (%s)",
		dbName, tableName, strings.Join(cols, ", "))

	if _, err := db.DB.Exec(statement); err != nil {
		log.Printf("CreateTable error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create table: " + err.Error()})
		return
	}

	// Record replication
	db.EnqueueReplication(statement)

	log.Printf("Table %s.%s created", dbName, tableName)
	c.JSON(http.StatusOK, gin.H{"message": "table created", "database": dbName, "table": tableName})
}

// ListTables returns a list of tables for a given database.
func ListTables(c *gin.Context) {
	dbName := sanitizeDBName(c.Param("dbname"))
	if dbName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid database name"})
		return
	}

	rows, err := db.DB.Query("SHOW TABLES FROM `" + dbName + "`")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	tables := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			tables = append(tables, name)
		}
	}
	c.JSON(http.StatusOK, gin.H{"database": dbName, "tables": tables})
}

func sanitizeIdentifier(s string) string {
	// For table/column names: allow alphanumeric and underscore
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
// GetTableSchema returns column names and types of a table
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

	// Convert []byte to string to stop base64 encoding
	name := ""
	if b, ok := field.([]byte); ok {
		name = string(b)
	} else {
		name = fmt.Sprintf("%v", field)
	}
	typeStr := ""
	if b, ok := colType.([]byte); ok {
		typeStr = string(b)
	} else {
		typeStr = fmt.Sprintf("%v", colType)
	}

	columns = append(columns, gin.H{
		"name": name,
		"type": typeStr,
	})
}
	c.JSON(http.StatusOK, gin.H{"columns": columns})
}
// DropTable removes a table from a database
func DropTable(c *gin.Context) {
	dbName := sanitizeDBName(c.Param("dbname"))
	tableName := sanitizeIdentifier(c.Param("table"))
	if dbName == "" || tableName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid database or table"})
		return
	}

	statement := fmt.Sprintf("DROP TABLE IF EXISTS `%s`.`%s`", dbName, tableName)
	if _, err := db.DB.Exec(statement); err != nil {
		log.Printf("DropTable error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not drop table"})
		return
	}

	// Record replication (so workers get it too)
	db.EnqueueReplication(statement)

	c.JSON(http.StatusOK, gin.H{"message": "table dropped", "database": dbName, "table": tableName})
}