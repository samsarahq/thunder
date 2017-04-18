package sqlgen

import (
	"sort"
	"testing"

	"github.com/kylelemons/godebug/pretty"
)

func TestMatcher(t *testing.T) {
	base := map[string]Filter{
		"bar": {"foo": "bar", "id": 1},
		"1":   {"id": 1},
		"baz": {"foo": "baz", "id": 1},
		"all": {},
	}

	rows := []map[string]interface{}{
		{"foo": "foo", "id": 0},
		{"foo": "foo", "id": 1},
		{"foo": "bar", "id": 1},
		{"id": 1},
	}

	// Compare the result of a matcher against a naive implementation that
	// iterates over all queries.
	m := newMatcher()
	queries := make(map[string]Filter)

	verify := func() {
		for _, row := range rows {
			var actual []string
			for _, name := range m.match(row) {
				actual = append(actual, name.(string))
			}
			sort.Strings(actual)

			var expected []string
			for name, filter := range queries {
				ok := true
				for k, v := range filter {
					if row[k] != v {
						ok = false
					}
				}
				if ok {
					expected = append(expected, name)
				}
			}
			sort.Strings(expected)

			if diff := pretty.Compare(actual, expected); diff != "" {
				t.Error(diff)
			}
		}
	}

	// Incrementally add and remove queries, verify correctness at every stage.
	for name, filter := range base {
		queries[name] = filter
		m.add(name, filter)
		verify()
	}

	for name, filter := range queries {
		m.remove(name, filter)
		delete(queries, name)
		verify()
	}

	// Ensure that groups get deleted once no more queries with that shape remain.
	if len(m.groups) != 0 {
		t.Error("expected cleanup")
	}
}
