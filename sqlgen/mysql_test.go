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
}

func TestCountQuery(t *testing.T) {
	testQuery(&countQuery{
		Table:   "foo",
		Where: &SimpleWhere{
			Columns: []string{"bar"},
			Values:  []interface{}{3},
		},
	}, "SELECT COUNT(*) FROM foo WHERE bar = ?", []interface{}{3}, t)

	testQuery(&countQuery{
		Table:   "foo2",
		Where: &SimpleWhere{
			Columns: []string{"baz"},
			Values:  []interface{}{"xyz"},
		},
	}, "SELECT COUNT(*) FROM foo2 WHERE baz = ?", []interface{}{"xyz"}, t)
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
