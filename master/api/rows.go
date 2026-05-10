package api

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"dist-db/master/db"
)

// InsertRow inserts a new row into a table.
// Accepts a JSON object with column-value pairs.
func InsertRow(c *gin.Context) {
	dbName := sanitizeDBName(c.Param("dbname"))
	tableName := sanitizeIdentifier(c.Param("table"))
	if dbName == "" || tableName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid database or table"})
		return
	}

	// Parse body as map
	var cols map[string]interface{}
	if err := c.ShouldBindJSON(&cols); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return
	}

	// Generate UUID for the id column
	id := uuid.New().String()
	cols["id"] = id

	// Build safe INSERT
	colNames := []string{}
	placeholders := []string{}
	values := []interface{}{}

	for col, val := range cols {
		col = sanitizeIdentifier(col)
		if col == "" {
			continue
		}
		colNames = append(colNames, "`"+col+"`")
		placeholders = append(placeholders, "?")
		values = append(values, val)
	}

	if len(colNames) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no columns provided"})
		return
	}

	statement := fmt.Sprintf("INSERT INTO `%s`.`%s` (%s) VALUES (%s)",
		dbName, tableName, strings.Join(colNames, ", "), strings.Join(placeholders, ", "))

	_, err := db.DB.Exec(statement, values...)
	if err != nil {
		log.Printf("InsertRow error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Record replication
	db.EnqueueReplication(statement, values...)

	log.Printf("Row inserted into %s.%s with id %s", dbName, tableName, id)
	c.JSON(http.StatusCreated, gin.H{"message": "row inserted", "id": id})
}

// SelectRows reads rows from a table.
// Query parameter: where (optional) e.g. ?where=id='...'
func SelectRows(c *gin.Context) {
	dbName := sanitizeDBName(c.Param("dbname"))
	tableName := sanitizeIdentifier(c.Param("table"))
	if dbName == "" || tableName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid database or table"})
		return
	}

	whereClause := c.Query("where")
	query := fmt.Sprintf("SELECT * FROM `%s`.`%s`", dbName, tableName)
	if whereClause != "" {
		query += " WHERE " + whereClause
	}

	rows, err := db.DB.Query(query)
	if err != nil {
		log.Printf("SelectRows error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	// Get column names
	cols, _ := rows.Columns()
	result := []map[string]interface{}{}
	for rows.Next() {
    vals := make([]interface{}, len(cols))
    valPtrs := make([]interface{}, len(cols))
    for i := range vals {
        valPtrs[i] = &vals[i]
    }

    if err := rows.Scan(valPtrs...); err != nil {
        continue
    }

    // Convert []byte to string so JSON encoding stays readable
    for i, v := range vals {
        if b, ok := v.([]byte); ok {
            vals[i] = string(b)
        }
    }

    rowMap := map[string]interface{}{}
    for i, col := range cols {
        rowMap[col] = vals[i]
    }
    result = append(result, rowMap)
}

	c.JSON(http.StatusOK, gin.H{"rows": result})
}

// UpdateRow modifies an existing row by its id.
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return
	}

	setClauses := []string{}
	values := []interface{}{}
	for col, val := range cols {
		col = sanitizeIdentifier(col)
		if col == "" {
			continue
		}
		setClauses = append(setClauses, fmt.Sprintf("`%s` = ?", col))
		values = append(values, val)
	}
	if len(setClauses) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no columns to update"})
		return
	}

	statement := fmt.Sprintf("UPDATE `%s`.`%s` SET %s WHERE `id` = ?",
		dbName, tableName, strings.Join(setClauses, ", "))
	values = append(values, id)

	res, err := db.DB.Exec(statement, values...)
	if err != nil {
		log.Printf("UpdateRow error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Record replication
	db.EnqueueReplication(statement, values...)

	affected, _ := res.RowsAffected()
	log.Printf("Updated %d row(s) in %s.%s (id=%s)", affected, dbName, tableName, id)
	c.JSON(http.StatusOK, gin.H{"message": "row updated", "affected": affected})
}

// DeleteRow removes a row by id.
func DeleteRow(c *gin.Context) {
	dbName := sanitizeDBName(c.Param("dbname"))
	tableName := sanitizeIdentifier(c.Param("table"))
	id := c.Param("id")
	if dbName == "" || tableName == "" || id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid parameters"})
		return
	}

	statement := "DELETE FROM `" + dbName + "`.`" + tableName + "` WHERE `id` = ?"
	res, err := db.DB.Exec(statement, id)
	if err != nil {
		log.Printf("DeleteRow error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Record replication
	db.EnqueueReplication(statement, id)

	affected, _ := res.RowsAffected()
	log.Printf("Deleted %d row(s) from %s.%s (id=%s)", affected, dbName, tableName, id)
	c.JSON(http.StatusOK, gin.H{"message": "row deleted", "affected": affected})
}