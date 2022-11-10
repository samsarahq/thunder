package sqlgen

import (
	"reflect"
	"testing"
)

type query interface {
	ToSQL() (string, []interface{})
}

func testQuery(q query, sql string, values []interface{}, t *testing.T) {
	actualSql, actualValues := q.ToSQL()
	if sql != actualSql || !reflect.DeepEqual(values, actualValues) {
		t.Errorf("%+v.ToSQL() = (%s, %v), expected (%s, %v)", q, actualSql, actualValues,
			sql, values)
	}
}

func TestSimpleWhere(t *testing.T) {
	testQuery(&SimpleWhere{
		Columns: []string{},
		Values:  []interface{}{},
	}, "", []interface{}{}, t)

	testQuery(&SimpleWhere{
		Columns: []string{"foo"},
		Values:  []interface{}{1},
	}, "foo = ?", []interface{}{1}, t)

	testQuery(&SimpleWhere{
		Columns: []string{"foo", "bar"},
		Values:  []interface{}{1, 2},
	}, "foo = ? AND bar = ?", []interface{}{1, 2}, t)

	testQuery(&SimpleWhere{
		Columns: []string{"foo", "bar", "baz"},
		Values:  []interface{}{1, 2, nil},
	}, "foo = ? AND bar = ? AND baz IS ?", []interface{}{1, 2, nil}, t)
}

func TestCountQuery(t *testing.T) {
	testQuery(&countQuery{
		Table: "foo",
		Where: &SimpleWhere{
			Columns: []string{"bar"},
			Values:  []interface{}{3},
		},
	}, "SELECT COUNT(*) FROM foo WHERE bar = ?", []interface{}{3}, t)

	testQuery(&countQuery{
		Table: "foo2",
		Where: &SimpleWhere{
			Columns: []string{"baz"},
			Values:  []interface{}{"xyz"},
		},
	}, "SELECT COUNT(*) FROM foo2 WHERE baz = ?", []interface{}{"xyz"}, t)

	testQuery(&countQuery{
		Table: "foo3",
		Where: &SimpleWhere{
			Columns: []string{"baz", "blah"},
			Values:  []interface{}{"xyz", nil},
		},
	}, "SELECT COUNT(*) FROM foo3 WHERE baz = ? AND blah IS ?", []interface{}{"xyz", nil}, t)
}

func TestSelectQuery(t *testing.T) {
	testQuery(&SelectQuery{
		Table:   "foo",
		Columns: []string{"bar"},
		Options: &SelectOptions{
			Where:  "bar = ?",
			Values: []interface{}{3},
		},
	}, "SELECT bar FROM foo WHERE bar = ?", []interface{}{3}, t)

	testQuery(&SelectQuery{
		Table:   "foo",
		Columns: []string{"bar"},
		Options: &SelectOptions{
			Where:  "bar = ? AND baz IS ?",
			Values: []interface{}{3, nil},
		},
	}, "SELECT bar FROM foo WHERE bar = ? AND baz IS ?", []interface{}{3, nil}, t)

	testQuery(&SelectQuery{
		Table:   "foo",
		Columns: []string{"bar", "baz"},
		Options: &SelectOptions{
			Values: []interface{}{},
		},
	}, "SELECT bar, baz FROM foo", []interface{}{}, t)

	testQuery(&SelectQuery{
		Table:   "foo",
		Columns: []string{"foo", "bar"},
		Options: &SelectOptions{
			Where:   "bar = ? AND baz LIKE ?",
			Values:  []interface{}{3, "xyz"},
			OrderBy: "bar",
			Limit:   20,
		},
	}, "SELECT foo, bar FROM foo WHERE bar = ? AND baz LIKE ? ORDER BY bar LIMIT 20", []interface{}{3, "xyz"}, t)

	testQuery(&SelectQuery{
		Table:   "foo",
		Columns: []string{"foo", "bar"},
		Options: &SelectOptions{
			OrderBy: "bar",
			Limit:   20,
		},
	}, "SELECT foo, bar FROM foo ORDER BY bar LIMIT 20", nil, t)

	testQuery(&SelectQuery{
		Table:   "foo",
		Columns: []string{"foo", "bar"},
		Options: &SelectOptions{
			OrderBy: "bar",
		},
	}, "SELECT foo, bar FROM foo ORDER BY bar", nil, t)

	testQuery(&SelectQuery{
		Table:   "foo",
		Columns: []string{"foo", "bar"},
		Options: &SelectOptions{
			Where:     "foo = ? AND bar = ?",
			Values:    []interface{}{25, "xyz"},
			ForUpdate: true,
		},
	}, "SELECT foo, bar FROM foo WHERE foo = ? AND bar = ? FOR UPDATE", []interface{}{25, "xyz"}, t)

	// FORCE INDEX with a single index
	testQuery(&SelectQuery{
		Table:   "foo",
		Columns: []string{"foo", "bar"},
		Options: &SelectOptions{
			Where:      "foo = ? AND bar = ?",
			Values:     []interface{}{25, "xyz"},
			OrderBy:    "bar",
			ForceIndex: []string{"best_index"},
		},
	}, "SELECT foo, bar FROM foo FORCE INDEX(best_index) WHERE foo = ? AND bar = ? ORDER BY bar", []interface{}{25, "xyz"}, t)

	// FORCE INDEX with multiple indexes
	testQuery(&SelectQuery{
		Table:   "foo",
		Columns: []string{"foo", "bar"},
		Options: &SelectOptions{
			Where:      "foo = ? AND bar = ?",
			Values:     []interface{}{25, "xyz"},
			OrderBy:    "bar",
			ForceIndex: []string{"best_index", "another_good_index"},
		},
	}, "SELECT foo, bar FROM foo FORCE INDEX(best_index,another_good_index) WHERE foo = ? AND bar = ? ORDER BY bar", []interface{}{25, "xyz"}, t)

	// USE INDEX with a single index
	testQuery(&SelectQuery{
		Table:   "foo",
		Columns: []string{"foo", "bar"},
		Options: &SelectOptions{
			Where:    "foo = ? AND bar = ?",
			Values:   []interface{}{25, "xyz"},
			OrderBy:  "bar",
			UseIndex: []string{"best_index"},
		},
	}, "SELECT foo, bar FROM foo USE INDEX(best_index) WHERE foo = ? AND bar = ? ORDER BY bar", []interface{}{25, "xyz"}, t)

	// USE INDEX with multiple indexes
	testQuery(&SelectQuery{
		Table:   "foo",
		Columns: []string{"foo", "bar"},
		Options: &SelectOptions{
			Where:    "foo = ? AND bar = ?",
			Values:   []interface{}{25, "xyz"},
			OrderBy:  "bar",
			UseIndex: []string{"best_index", "another_good_index"},
		},
	}, "SELECT foo, bar FROM foo USE INDEX(best_index,another_good_index) WHERE foo = ? AND bar = ? ORDER BY bar", []interface{}{25, "xyz"}, t)

	// Both USE INDEX and FORCE INDEX are specified: only FORCE INDEX is applied
	testQuery(&SelectQuery{
		Table:   "foo",
		Columns: []string{"foo", "bar"},
		Options: &SelectOptions{
			Where:      "foo = ? AND bar = ?",
			Values:     []interface{}{25, "xyz"},
			OrderBy:    "bar",
			UseIndex:   []string{"good_index", "another_good_index"},
			ForceIndex: []string{"best_index"},
		},
	}, "SELECT foo, bar FROM foo FORCE INDEX(best_index) WHERE foo = ? AND bar = ? ORDER BY bar", []interface{}{25, "xyz"}, t)
}

func TestInsertQuery(t *testing.T) {
	testQuery(&InsertQuery{
		Table:   "foo",
		Columns: []string{"bar"},
		Values:  []interface{}{3},
	}, "INSERT INTO foo (bar) VALUES (?)", []interface{}{3}, t)

	testQuery(&InsertQuery{
		Table:   "foo2",
		Columns: []string{"bar", "baz"},
		Values:  []interface{}{3, "buh"},
	}, "INSERT INTO foo2 (bar, baz) VALUES (?, ?)", []interface{}{3, "buh"}, t)

	testQuery(&InsertQuery{
		Table:   "foo3",
		Columns: []string{},
		Values:  []interface{}{},
	}, "INSERT INTO foo3", []interface{}{}, t)
}

func TestBatchInsertQuery(t *testing.T) {
	testQuery(&BatchInsertQuery{
		Table:   "foo",
		Columns: []string{"bar"},
		Values:  []interface{}{3, 4},
	}, "INSERT INTO foo (bar) VALUES (?), (?)", []interface{}{3, 4}, t)

	testQuery(&BatchInsertQuery{
		Table:   "foo2",
		Columns: []string{"bar", "baz"},
		Values:  []interface{}{3, "buh"},
	}, "INSERT INTO foo2 (bar, baz) VALUES (?, ?)", []interface{}{3, "buh"}, t)

	testQuery(&BatchInsertQuery{
		Table:   "foo2",
		Columns: []string{"bar", "baz"},
		Values:  []interface{}{3, "buh", 5, "test"},
	}, "INSERT INTO foo2 (bar, baz) VALUES (?, ?), (?, ?)", []interface{}{3, "buh", 5, "test"}, t)
}

func TestUpsertQuery(t *testing.T) {
	testQuery(&UpsertQuery{
		Table:   "foo",
		Columns: []string{"bar"},
		Values:  []interface{}{10},
	}, "INSERT INTO foo (bar) VALUES (?) ON DUPLICATE KEY UPDATE bar=VALUES(bar)", []interface{}{10}, t)

	testQuery(&UpsertQuery{
		Table:   "foo",
		Columns: []string{"bar", "baz"},
		Values:  []interface{}{10, "buh"},
	}, "INSERT INTO foo (bar, baz) VALUES (?, ?) ON DUPLICATE KEY UPDATE bar=VALUES(bar), baz=VALUES(baz)", []interface{}{10, "buh"}, t)
}

func TestBatchUpsertQuery(t *testing.T) {
	testQuery(&BatchUpsertQuery{
		Table:   "foo",
		Columns: []string{"bar"},
		Values:  []interface{}{3, 4},
	}, "INSERT INTO foo (bar) VALUES (?), (?) ON DUPLICATE KEY UPDATE bar=VALUES(bar)", []interface{}{3, 4}, t)

	testQuery(&BatchUpsertQuery{
		Table:   "foo2",
		Columns: []string{"bar", "baz"},
		Values:  []interface{}{3, "buh"},
	}, "INSERT INTO foo2 (bar, baz) VALUES (?, ?) ON DUPLICATE KEY UPDATE bar=VALUES(bar), baz=VALUES(baz)", []interface{}{3, "buh"}, t)

	testQuery(&BatchUpsertQuery{
		Table:   "foo2",
		Columns: []string{"bar", "baz"},
		Values:  []interface{}{3, "buh", 5, "test"},
	}, "INSERT INTO foo2 (bar, baz) VALUES (?, ?), (?, ?) ON DUPLICATE KEY UPDATE bar=VALUES(bar), baz=VALUES(baz)", []interface{}{3, "buh", 5, "test"}, t)
}

func TestUpdateQuery(t *testing.T) {
	testQuery(&UpdateQuery{
		Table:   "foo",
		Columns: []string{"bar"},
		Values:  []interface{}{10},
		Where: &SimpleWhere{
			Columns: []string{"bar"},
			Values:  []interface{}{3},
		},
	}, "UPDATE foo SET bar = ? WHERE bar = ?", []interface{}{10, 3}, t)

	testQuery(&UpdateQuery{
		Table:   "foo2",
		Columns: []string{"bar", "baz"},
		Values:  []interface{}{10, "xyz"},
		Where: &SimpleWhere{
			Columns: []string{"bar"},
			Values:  []interface{}{3},
		},
	}, "UPDATE foo2 SET bar = ?, baz = ? WHERE bar = ?", []interface{}{10, "xyz", 3}, t)
}

func TestDeleteQuery(t *testing.T) {
	testQuery(&DeleteQuery{
		Table: "foo",
		Where: &SimpleWhere{
			Columns: []string{"bar"},
			Values:  []interface{}{3},
		},
	}, "DELETE FROM foo WHERE bar = ?", []interface{}{3}, t)

	testQuery(&DeleteQuery{
		Table: "foo",
		Where: &SimpleWhere{
			Columns: []string{},
			Values:  []interface{}{},
		},
	}, "DELETE FROM foo", []interface{}{}, t)
}
