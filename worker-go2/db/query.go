package db


// QueryRowMap executes a query and returns all rows as []map[string]interface{}
func QueryRowMap(query string, args ...interface{}) ([]map[string]interface{}, error) {
	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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
	return result, nil
}

// ExecAffected executes a non-select query and returns rows affected.
func ExecAffected(query string, args ...interface{}) (int64, error) {
	res, err := DB.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}