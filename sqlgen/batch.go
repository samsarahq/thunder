package sqlgen

import (
	"bytes"
	"sort"
)

// makeBatchQuery combines a set of filters into a single SQL that matches any
// of the filters, in order to fulfill many independent queries with a single
// SQL SELECT.
//
// Different filters with equal sets of columns are combined into grouped IN
// queries. For example, the filters
//   {"id": 10}
//   {"id": 20}
//   {"name": "Bob", "city": "San Francisco"}
// will get combined to form the query
//   WHERE ((city, name)) in ("San Francisco", "Bob") OR id in (10, 20)
// (except with parameter substitution.)
func makeBatchQuery(filters []Filter) (string, []interface{}) {
	// A batchQueryGroup holds all value tuples for a given set of columns in the
	// WHERE statement.
	type batchQueryGroup struct {
		columns []string
		tuples  [][]interface{}
	}

	// Put every filter in its group.
	groups := make(map[string]*batchQueryGroup)
	for _, filter := range filters {
		// If any of the filters is empty (and matches all rows), short-circuit and
		// return an empty WHERE clause. This is special case logic because you can't
		// create an IN query with 0 columns.
		if len(filter) == 0 {
			return "", nil
		}

		// Figure out this filters' group.
		columns := extractColumns(filter)
		key := columnsKey(columns)

		// Maybe create the group.
		group, ok := groups[key]
		if !ok {
			group = &batchQueryGroup{
				columns: columns,
			}
			groups[key] = group
		}

		// Add the filter to the group.
		group.tuples = append(group.tuples, extractValuesTuple(filter, columns))
	}

	// Sort the groups by their key (for deterministic tests.)
	groupKeys := make([]string, 0, len(groups))
	for group := range groups {
		groupKeys = append(groupKeys, group)
	}
	sort.Strings(groupKeys)

	// Build the WHERE clause one group at a time.
	var clause bytes.Buffer
	var args []interface{}
	for i, key := range groupKeys {
		group := groups[key]

		// Insert OR between groups.
		if i > 0 {
			clause.WriteString(" OR ")
		}

		// Print the group's columns, and build a (?, ?, ?) string for the tuples.
		var options bytes.Buffer
		if len(group.columns) > 1 {
			clause.WriteString("(")
			options.WriteString("(")
		}
		for i, column := range group.columns {
			if i > 0 {
				clause.WriteString(", ")
				options.WriteString(", ")
			}
			clause.WriteString(column)
			options.WriteString("?")
		}
		if len(group.columns) > 1 {
			clause.WriteString(")")
			options.WriteString(")")
		}

		// Print the group's tuples.
		clause.WriteString(" IN (")
		for j, tuple := range group.tuples {
			// Separate tuples with commas.
			if j > 0 {
				clause.WriteString(", ")
			}
			// Write (?, ?, ?) string for the tuple, and append the arguments.
			clause.Write(options.Bytes())
			args = append(args, tuple...)
		}
		clause.WriteString(")")
	}

	return clause.String(), args
}
