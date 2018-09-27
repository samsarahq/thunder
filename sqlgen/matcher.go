package sqlgen

import (
	"sort"
	"strings"

	"github.com/samsarahq/thunder/internal"
)

// extractColumns extracts the columns used in a filter. For example, for
// Filter{"id": 10, "name": "bob"}, the columns are []string{"id", "name"}.
func extractColumns(filter Filter) []string {
	columns := make([]string, 0, len(filter))
	for column := range filter {
		columns = append(columns, column)
	}
	// Sort the columns to obtain a canonical ordering for these columns,
	// to get consistent column keys and value tuples.
	sort.Strings(columns)
	return columns
}

// extractValuesTuple extracts the values of a set of columns in a filter. For
// example, for Filter{"id": 10, "name": "bob"} the values are
// []interface{}{10, "bob"}.
func extractValuesTuple(m map[string]interface{}, columns []string) []interface{} {
	values := make([]interface{}, 0, len(columns))
	for _, column := range columns {
		values = append(values, m[column])
	}
	return values
}

// columnsKey builds a string key for a set of columns useable in a map. For example,
// for Filter{"id": 10, "name": "bob"}, the key is "id;name".
func columnsKey(columns []string) string {
	return strings.Join(columns, ";")
}

// A matcher efficiently matches table rows against sets of queries. The
// intended use case is for logic that tracks a lot of queries (eg. the binlog
// change tracker, or the query batcher), and that wants to match rows against
// those queries. Matcher queries are sets of equality constraints on some
// fields, such as id=10, id=20, and (city="sf" AND name="bob").
//
// Naively, to match a row against these queries, you would have to test all
// queries, which is inefficient for large sets of queries.
//
// Instead, the matcher uses the fact that the queries id=10 and id=20 have the
// same shape to optimize matching them: The matcher extracts the common set of
// columns, (id), and the specific values matched by each query, (10) and (20).
// Then, to match a query, the matcher extracts the id from that query, and
// looks it up in a lookup table from tuples to query. Here, that lookup table
// will hold (10) and (20) associated with some opaque id (such as a *query
// pointer, or some index in array) to identify the queries.
//
// For every different query shape, the matcher builds a *matcherGroup holding
// a lookup table, letting the matcher process queries in time O(#query shapes).
type matcher struct {
	groups map[string]*matcherGroup
}

// A matcherGroup holds a set of tuples for a given set of columns in a
// matcher.
type matcherGroup struct {
	columns []string
	// queriesByTuples is a map from column values to sets of queries interested in those
	// specific values. For example, if columns is []string{"city", "name"},
	// tuples is a map from tuples such as [2]interface{}{"sf", "bob"} and
	// [2]interface{}{"sf", "alice"} to sets of opaque query handles.
	queriesByTuple map[interface{}]map[interface{}]struct{}
}

// newMatcher creates a new matcher.
func newMatcher() *matcher {
	return &matcher{
		groups: make(map[string]*matcherGroup),
	}
}

// add adds a query to the matcher, associating an opaque id with the query.
func (m *matcher) add(id interface{}, filter Filter) {
	columnSet := extractColumns(filter)
	tuple := internal.MakeHashable(extractValuesTuple(filter, columnSet))
	key := columnsKey(columnSet)

	g, ok := m.groups[key]
	if !ok {
		g = &matcherGroup{
			columns:        columnSet,
			queriesByTuple: make(map[interface{}]map[interface{}]struct{}),
		}
		m.groups[key] = g
	}

	queries, ok := g.queriesByTuple[tuple]
	if !ok {
		queries = make(map[interface{}]struct{})
		g.queriesByTuple[tuple] = queries
	}

	queries[id] = struct{}{}
}

// removes removes a query from the matcher.
func (m *matcher) remove(id interface{}, filter Filter) {
	columnSet := extractColumns(filter)
	tuple := internal.MakeHashable(extractValuesTuple(filter, columnSet))
	key := columnsKey(columnSet)

	g, ok := m.groups[key]
	if !ok {
		return
	}

	queriesByTuple, ok := g.queriesByTuple[tuple]
	if !ok {
		return
	}

	delete(queriesByTuple, id)
	if len(queriesByTuple) > 0 {
		return
	}

	delete(g.queriesByTuple, tuple)
	if len(g.queriesByTuple) > 0 {
		return
	}

	delete(m.groups, key)
}

// match matches an object against all queries and returns all matching query
// ids.
func (m *matcher) match(o map[string]interface{}) []interface{} {
	var result []interface{}
	for _, g := range m.groups {
		for query := range g.queriesByTuple[internal.MakeHashable(extractValuesTuple(o, g.columns))] {
			result = append(result, query)
		}
	}
	return result
}
