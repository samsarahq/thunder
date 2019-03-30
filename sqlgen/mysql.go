package sqlgen

import (
	"bytes"
	"fmt"
)

// SimpleWhere represents a simple WHERE clause
type SimpleWhere struct {
	Columns []string
	Values  []interface{}
}

// ToSQL builds a `a = ? AND b = ?` clause
func (w *SimpleWhere) ToSQL() (string, []interface{}) {
	var buffer bytes.Buffer

	if len(w.Columns) > 0 {
		for i, column := range w.Columns {
			if i > 0 {
				buffer.WriteString(" AND ")
			}
			buffer.WriteString(column)
			buffer.WriteString(" = ?")
		}
	}

	return buffer.String(), w.Values
}

type SQLQuery interface {
	ToSQL() (string, []interface{})
}

type countQuery struct {
	Table string
	Where *SimpleWhere
}

// ToSQL builds a parameterized SELECT COUNT(*) FROM x ... statement
func (q *countQuery) ToSQL() (string, []interface{}) {
	var buffer bytes.Buffer

	buffer.WriteString("SELECT COUNT(*)")
	buffer.WriteString(" FROM ")
	buffer.WriteString(q.Table)

	where, whereValues := q.Where.ToSQL()
	if where != "" {
		buffer.WriteString(" WHERE ")
		buffer.WriteString(where)
	}

	return buffer.String(), whereValues
}

// SelectQuery represents a SELECT query
type SelectQuery struct {
	Table   string
	Columns []string

	Options *SelectOptions
}

// ToSQL builds a parameterized SELECT a, b, c FROM x ... statement
func (q *SelectQuery) ToSQL() (string, []interface{}) {
	var buffer bytes.Buffer

	buffer.WriteString("SELECT ")
	for i, column := range q.Columns {
		if i > 0 {
			buffer.WriteString(", ")
		}
		buffer.WriteString(column)
	}
	buffer.WriteString(" FROM ")
	buffer.WriteString(q.Table)

	if q.Options.Where != "" {
		buffer.WriteString(" WHERE ")
		buffer.WriteString(q.Options.Where)
	}

	if q.Options.OrderBy != "" {
		buffer.WriteString(" ORDER BY ")
		buffer.WriteString(q.Options.OrderBy)
	}

	if q.Options.Limit != 0 {
		buffer.WriteString(" LIMIT ")
		fmt.Fprint(&buffer, q.Options.Limit)
	}

	return buffer.String(), q.Options.Values
}

// InsertQuery represents a INSERT query
type InsertQuery struct {
	Table   string
	Columns []string
	Values  []interface{}
}

// ToSQL builds a parameterized INSERT INTO x (a, b) VALUES (?, ?) statement
func (q *InsertQuery) ToSQL() (string, []interface{}) {
	var buffer bytes.Buffer
	buffer.WriteString("INSERT INTO ")
	buffer.WriteString(q.Table)

	if len(q.Columns) > 0 {
		buffer.WriteString(" (")
		for i, column := range q.Columns {
			if i > 0 {
				buffer.WriteString(", ")
			}
			buffer.WriteString(column)
		}
		buffer.WriteString(") VALUES (")
		for i := range q.Columns {
			if i > 0 {
				buffer.WriteString(", ")
			}
			buffer.WriteString("?")
		}
		buffer.WriteString(")")
	}

	return buffer.String(), q.Values
}

// UpsertQuery represents a INSERT ... ON DUPLICATE KEY UPDATE query
type UpsertQuery struct {
	Table   string
	Columns []string
	Values  []interface{}
}

// ToSQL builds a parameterized INSERT INTO x (a, b) VALUES (?, ?) statement
func (q *UpsertQuery) ToSQL() (string, []interface{}) {
	var buffer bytes.Buffer
	buffer.WriteString("INSERT INTO ")
	buffer.WriteString(q.Table)

	buffer.WriteString(" (")
	for i, column := range q.Columns {
		if i > 0 {
			buffer.WriteString(", ")
		}
		buffer.WriteString(column)
	}
	buffer.WriteString(") VALUES (")
	for i := range q.Columns {
		if i > 0 {
			buffer.WriteString(", ")
		}
		buffer.WriteString("?")
	}
	buffer.WriteString(") ON DUPLICATE KEY UPDATE ")
	for i, column := range q.Columns {
		if i > 0 {
			buffer.WriteString(", ")
		}
		buffer.WriteString(column)
		buffer.WriteString("=VALUES(")
		buffer.WriteString(column)
		buffer.WriteString(")")
	}

	return buffer.String(), q.Values
}

// UpdateQuery represents a UPDATE query
type UpdateQuery struct {
	Table   string
	Columns []string
	Values  []interface{}
	Where   *SimpleWhere
}

// ToSQL builds a parameterized UPDATE x SET a = ?, b = ? WHERE c = ? statement
func (q *UpdateQuery) ToSQL() (string, []interface{}) {
	var buffer bytes.Buffer
	buffer.WriteString("UPDATE ")
	buffer.WriteString(q.Table)

	if len(q.Columns) > 0 {
		buffer.WriteString(" SET ")
		for i, column := range q.Columns {
			if i > 0 {
				buffer.WriteString(", ")
			}
			buffer.WriteString(column)
			buffer.WriteString(" = ?")
		}
	}

	where, whereValues := q.Where.ToSQL()
	if where != "" {
		buffer.WriteString(" WHERE ")
		buffer.WriteString(where)
	}

	values := make([]interface{}, len(q.Values)+len(whereValues))
	copy(values, q.Values)
	copy(values[len(q.Values):], whereValues)

	return buffer.String(), values
}

// DeleteQuery represents a DELETE query
type DeleteQuery struct {
	Table string
	Where *SimpleWhere
}

// ToSQL builds a parameterized DELETE FROM x WHERE a = ? AND b = ? statement
func (q *DeleteQuery) ToSQL() (string, []interface{}) {
	var buffer bytes.Buffer
	buffer.WriteString("DELETE FROM ")
	buffer.WriteString(q.Table)

	where, values := q.Where.ToSQL()
	if where != "" {
		buffer.WriteString(" WHERE ")
		buffer.WriteString(where)
	}

	return buffer.String(), values
}
