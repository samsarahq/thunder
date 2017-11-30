package sqlgen

import (
	"testing"

	"github.com/kylelemons/godebug/pretty"
)

func TestMakeBatchQuery(t *testing.T) {
	testcases := []struct {
		Title   string
		Filters []Filter
		Clause  string
		Args    []interface{}
	}{
		{
			Title: "Simple IDs",
			Filters: []Filter{
				{"id": 10},
				{"id": 20},
				{"id": 30},
			},
			Clause: "id IN (?, ?, ?)",
			Args:   []interface{}{10, 20, 30},
		},
		{
			Title: "Simple compound IDs",
			Filters: []Filter{
				{"id_a": 10, "id_b": "foo"},
				{"id_a": 20, "id_b": "bar"},
				{"id_a": 30, "id_b": "baz"},
			},
			Clause: "(id_a=? AND id_b=?) OR (id_a=? AND id_b=?) OR (id_a=? AND id_b=?)",
			Args:   []interface{}{10, "foo", 20, "bar", 30, "baz"},
		},
		{
			Title: "Empty filter",
			Filters: []Filter{
				{},
				{"id": 20},
			},
			Clause: "",
			Args:   nil,
		},
		{
			Title: "Simple and compound mixed",
			Filters: []Filter{
				{"id": 10},
				{"id": 20},
				{"id": 30},
				{"id_a": 10, "id_b": "foo"},
				{"id_a": 20, "id_b": "bar"},
				{"id_a": 30, "id_b": "baz"},
			},
			Clause: "id IN (?, ?, ?) OR (id_a=? AND id_b=?) OR (id_a=? AND id_b=?) OR (id_a=? AND id_b=?)",
			Args:   []interface{}{10, 20, 30, 10, "foo", 20, "bar", 30, "baz"},
		},
	}

	for _, testcase := range testcases {
		clause, args := makeBatchQuery(testcase.Filters)
		if clause != testcase.Clause {
			t.Errorf("%s clause: got %s, expected %s", testcase.Title, clause, testcase.Clause)
		}
		if diff := pretty.Compare(args, testcase.Args); diff != "" {
			t.Errorf("%s args: diff %s", testcase.Title, diff)
		}
	}
}
