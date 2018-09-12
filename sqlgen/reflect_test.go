package sqlgen

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/kylelemons/godebug/pretty"
	"github.com/samsarahq/thunder/fields"
	"github.com/stretchr/testify/assert"
)

func testMakeSnake(t *testing.T, s, expected string) {
	actual := makeSnake(s)
	if actual != expected {
		t.Errorf("makeSnake(%s) = %s, expected %s", s, actual, expected)
	}
}

func TestMakeSnake(t *testing.T) {
	testMakeSnake(t, "FooBar", "foo_bar")
	testMakeSnake(t, "OrganizationId", "organization_id")
	testMakeSnake(t, "ABC", "a_b_c")
}

func TestCopySlice(t *testing.T) {
	one := int64(1)
	two := int64(2)
	three := int64(3)

	var result []*int64
	src := []interface{}{
		nil, &one, nil, &two, nil, &three, nil,
	}
	if err := CopySlice(&result, src); err != nil {
		t.Error(err)
	}
	expected := []*int64{
		nil, &one, nil, &two, nil, &three, nil,
	}
	if diff := pretty.Compare(expected, result); diff != "" {
		t.Errorf("Unexpected result from CopySlice: %s", diff)
	}
}

type alias int64

type simple struct {
	A     int64  `sql:"a,primary"`
	FooId string `sql:",primary"`
	d     string
	C     float64 `sql:"column"`
	D     *int64
	E     alias
}

type noprimary struct {
	A int64
}

type anonymous struct {
	simple
}

type duplicate struct {
	A int64
	B int64 `sql:"a"`
}

func TestRegisterType(t *testing.T) {
	s := NewSchema()
	if err := s.RegisterType("simple", AutoIncrement, simple{}); err != nil {
		t.Fatal(err)
	}

	if err := s.RegisterType("a", AutoIncrement, noprimary{}); err == nil {
		t.Error("expected no primary to fail")
	}

	if err := s.RegisterType("b", AutoIncrement, 1); err == nil {
		t.Error("expected int to fail")
	}

	if err := s.RegisterType("c", AutoIncrement, &simple{}); err == nil {
		t.Error("expected pointer to struct to fail")
	}

	if err := s.RegisterType("d", AutoIncrement, duplicate{}); err == nil {
		t.Error("expected duplicate fields to fail")
	}

	if err := s.RegisterType("e", AutoIncrement, anonymous{}); err == nil {
		t.Error("expected anonymous fields to fail")
	}
}

type user struct {
	Id       int64 `sql:",primary"`
	Name     string
	Age      int64
	Optional *string
}

type IntAlias int64
type SuffixString string

func (c *SuffixString) Scan(value interface{}) error {
	switch value := value.(type) {
	case nil:
		*c = SuffixString("")
	case string:
		*c = SuffixString(value + "-FOO")
	default:
		return fmt.Errorf("cannot convert %v to string", value)
	}
	return nil
}

var tmp = SuffixString("")
var _ sql.Scanner = &tmp

func (c SuffixString) Value() (driver.Value, error) {
	return strings.TrimSuffix(string(c), "-FOO"), nil
}

var _ driver.Valuer = SuffixString("")

type custom struct {
	Id           int64 `sql:",primary"`
	IntAlias     IntAlias
	SuffixString SuffixString
}

func fieldFromValue(i interface{}) *fields.Descriptor {
	return fields.New(reflect.TypeOf(i), nil)
}

func TestMakeWhere(t *testing.T) {
	s := NewSchema()
	if err := s.RegisterType("users", AutoIncrement, user{}); err != nil {
		t.Fatal(err)
	}
	table := s.ByName["users"]

	where, err := makeWhere(table, Filter{"id": 10})
	assert.NoError(t, err)
	assert.Equal(t, []string{"id"}, where.Columns)
	assertSameValues(t, []interface{}{int64(10)}, where.Values)

	where, err = makeWhere(table, Filter{"id": 10, "name": "bob", "age": 30})
	assert.NoError(t, err)
	assert.Equal(t, []string{"id", "name", "age"}, where.Columns)
	assertSameValues(t, []interface{}{int64(10), "bob", int64(30)}, where.Values)

	where, err = makeWhere(table, Filter{})
	assert.NoError(t, err)
	assert.Equal(t, &SimpleWhere{Columns: []string{}, Values: []interface{}{}}, where)

	_, err = makeWhere(table, Filter{"foo": "bar"})
	if err == nil {
		t.Error("expected error with unknown field")
	}
}

func TestMakeSelect(t *testing.T) {
	s := NewSchema()
	if err := s.RegisterType("users", AutoIncrement, user{}); err != nil {
		t.Fatal(err)
	}

	var users []*user
	query, err := s.MakeSelect(&users, Filter{"id": 10}, nil)
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(query, &BaseSelectQuery{
		Table:  s.ByName["users"],
		Filter: Filter{"id": 10},
	}) {
		t.Error("bad select")
	}

	query, err = s.MakeSelect(&users, Filter{}, nil)
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(query, &BaseSelectQuery{
		Table:  s.ByName["users"],
		Filter: Filter{},
	}) {
		t.Error("bad select")
	}

	query, err = s.MakeSelect(&users, Filter{"name": "bob", "age": 10}, nil)
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(query, &BaseSelectQuery{
		Table:  s.ByName["users"],
		Filter: Filter{"name": "bob", "age": 10},
	}) {
		t.Error("bad select")
	}
}

func TestSelectOptions(t *testing.T) {
	s := NewSchema()
	if err := s.RegisterType("users", AutoIncrement, user{}); err != nil {
		t.Fatal(err)
	}

	var users []*user
	query, err := s.MakeSelect(&users, Filter{"id": 10}, &SelectOptions{
		Where:   "name LIKE ?",
		Values:  []interface{}{"abc%"},
		OrderBy: "name",
		Limit:   20,
	})
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(query, &BaseSelectQuery{
		Table:  s.ByName["users"],
		Filter: Filter{"id": 10},
		Options: &SelectOptions{
			Where:   "name LIKE ?",
			Values:  []interface{}{"abc%"},
			OrderBy: "name",
			Limit:   20,
		},
	}) {
		spew.Dump(query)
		t.Error("bad select")
	}

	query, err = s.MakeSelect(&users, nil, &SelectOptions{
		Where: "1=2",
	})
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(query, &BaseSelectQuery{
		Table: s.ByName["users"],
		Options: &SelectOptions{
			Where: "1=2",
		},
	}) {
		t.Error("bad select")
	}

	query, err = s.MakeSelect(&users, Filter{"name": "bob", "age": 10}, &SelectOptions{
		OrderBy: "age",
		Limit:   5,
	})
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(query, &BaseSelectQuery{
		Table:  s.ByName["users"],
		Filter: Filter{"name": "bob", "age": 10},
		Options: &SelectOptions{
			OrderBy: "age",
			Limit:   5,
		},
	}) {
		t.Error("bad select")
	}
}

func TestMakeSelectRow(t *testing.T) {
	s := NewSchema()
	if err := s.RegisterType("users", AutoIncrement, user{}); err != nil {
		t.Fatal(err)
	}

	var u *user
	query, err := s.MakeSelectRow(&u, Filter{"name": "alice"}, nil)
	assert.NoError(t, err)
	assert.Equal(t, &BaseSelectQuery{
		Table:  s.ByName["users"],
		Filter: Filter{"name": "alice"},
	}, query)
}

func assertSameValues(t *testing.T, expected []interface{}, got []interface{}) {
	if len(expected) != len(got) {
		t.Errorf("Mistmatched values length")
		return
	}

	for i := range expected {
		valuer := got[i].(fields.Valuer)
		val, err := valuer.Value()
		assert.NoError(t, err)
		assert.Equal(t, expected[i], val)
	}
}

func TestMakeInsertAutoIncrement(t *testing.T) {
	s := NewSchema()
	if err := s.RegisterType("users", AutoIncrement, user{}); err != nil {
		t.Fatal(err)
	}

	query, err := s.MakeInsertRow(&user{
		Name: "bob",
		Age:  20,
	})
	assert.NoError(t, err)
	assert.Equal(t, "users", query.Table)
	assert.Equal(t, []string{"name", "age", "optional"}, query.Columns)
	assertSameValues(t, []interface{}{"bob", int64(20), nil}, query.Values)
}

func TestMakeUpsertAutoIncrement(t *testing.T) {
	s := NewSchema()
	if err := s.RegisterType("users", AutoIncrement, user{}); err != nil {
		t.Fatal(err)
	}

	_, err := s.MakeUpsertRow(&user{
		Name: "bob",
		Age:  20,
	})
	if err == nil || !strings.Contains(err.Error(), "upsert only supports unique value primary keys") {
		t.Errorf("expected failure upserting autoincrement, got %s", err)
	}
}

func TestMakeUpsertUniqueId(t *testing.T) {
	s := NewSchema()
	if err := s.RegisterType("users", UniqueId, user{}); err != nil {
		t.Fatal(err)
	}

	var temp = "temp"
	query, err := s.MakeUpsertRow(&user{
		Id:       5,
		Name:     "alice",
		Age:      30,
		Optional: &temp,
	})
	assert.NoError(t, err)
	assert.Equal(t, "users", query.Table)
	assert.Equal(t, []string{"id", "name", "age", "optional"}, query.Columns)
	assertSameValues(t, []interface{}{int64(5), "alice", int64(30), "temp"}, query.Values)
}

func TestMakeUpdateAutoIncrement(t *testing.T) {
	s := NewSchema()
	if err := s.RegisterType("users", AutoIncrement, user{}); err != nil {
		t.Fatal(err)
	}

	query, err := s.MakeUpdateRow(&user{
		Id:   10,
		Name: "bob",
		Age:  20,
	})
	assert.NoError(t, err)
	assert.Equal(t, "users", query.Table)
	assert.Equal(t, []string{"name", "age", "optional"}, query.Columns)
	assertSameValues(t, []interface{}{"bob", int64(20), nil}, query.Values)
	assert.Equal(t, []string{"id"}, query.Where.Columns)
	assertSameValues(t, []interface{}{int64(10)}, query.Where.Values)
}

func TestMakeUpdateUniqueId(t *testing.T) {
	s := NewSchema()
	if err := s.RegisterType("users", UniqueId, user{}); err != nil {
		t.Fatal(err)
	}

	var temp = "temp"
	query, err := s.MakeUpdateRow(&user{
		Id:       20,
		Name:     "alice",
		Age:      40,
		Optional: &temp,
	})
	assert.NoError(t, err)
	assert.Equal(t, "users", query.Table)
	assert.Equal(t, []string{"name", "age", "optional"}, query.Columns)
	assertSameValues(t, []interface{}{"alice", int64(40), "temp"}, query.Values)
	assert.Equal(t, []string{"id"}, query.Where.Columns)
	assertSameValues(t, []interface{}{int64(20)}, query.Where.Values)
}

func TestMakeDelete(t *testing.T) {
	s := NewSchema()
	if err := s.RegisterType("users", AutoIncrement, user{}); err != nil {
		t.Fatal(err)
	}

	query, err := s.MakeDeleteRow(&user{
		Id:   10,
		Name: "bob",
		Age:  20,
	})
	assert.NoError(t, err)
	assert.Equal(t, "users", query.Table)
	assert.Equal(t, []string{"id"}, query.Where.Columns)
	assertSameValues(t, []interface{}{int64(10)}, query.Where.Values)
}

func TestCoerce(t *testing.T) {
	ten := int64(10)
	foo := "foo"

	cases := []struct {
		Description string
		Input       interface{}
		Expected    interface{}
	}{
		{Description: "int64", Input: int64(10), Expected: int64(10)},
		{Description: "*int64", Input: &ten, Expected: int64(10)},
		{Description: "foo", Input: "foo", Expected: "foo"},
		{Description: "*foo", Input: &foo, Expected: "foo"},
		{Description: "nil", Input: nil, Expected: nil},
		{Description: "(*int64)(nil)", Input: (*int64)(nil), Expected: nil},
		{Description: "(*string)(nil)", Input: (*string)(nil), Expected: nil},
	}

	for _, c := range cases {
		actual := coerce(reflect.ValueOf(c.Input))
		if actual != c.Expected {
			t.Errorf("%s: got %v, expected %v", c.Description, actual, c.Expected)
		}
	}
}

func TestMakeTester(t *testing.T) {
	s := NewSchema()
	if err := s.RegisterType("users", AutoIncrement, user{}); err != nil {
		t.Fatal(err)
	}

	ten := int64(10)

	idTen, err := s.MakeTester("users", Filter{"id": int64(10)})
	if err != nil {
		t.Error(err)
	}

	idTenOptionalNil, err := s.MakeTester("users", Filter{
		"id":       &ten,
		"optional": (*string)(nil),
	})
	if err != nil {
		t.Error(err)
	}

	idTenOptionalFoo, err := s.MakeTester("users", Filter{
		"id":       int64(10),
		"optional": "foo",
	})
	if err != nil {
		t.Error(err)
	}

	foo := "foo"

	cases := []struct {
		Description string
		Tester      Tester
		User        *user
		Expected    bool
	}{
		{Description: "compare int match", Tester: idTen, User: &user{Id: 10}, Expected: true},
		{Description: "compare int fail", Tester: idTen, User: &user{Id: 5}, Expected: false},
		{Description: "compare nil match", Tester: idTenOptionalNil, User: &user{Id: 10}, Expected: true},
		{Description: "compare nil fail", Tester: idTenOptionalNil, User: &user{Id: 10, Optional: &foo}, Expected: false},
		{Description: "compare ptr match", Tester: idTenOptionalFoo, User: &user{Id: 10, Optional: &foo}, Expected: true},
		{Description: "compare ptr fail", Tester: idTenOptionalFoo, User: &user{Id: 10}, Expected: false},
	}

	for _, c := range cases {
		actual := c.Tester.Test(c.User)
		if actual != c.Expected {
			t.Errorf("%s: got %v, expected %v", c.Description, actual, c.Expected)
		}
	}
}
