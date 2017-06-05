package sqlgen

import (
	"database/sql/driver"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/kylelemons/godebug/pretty"
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

type unsupported struct {
	A byte
}

func TestRegisterType(t *testing.T) {
	s := NewSchema()
	s.RegisterSimpleScalar(alias(0))
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

	if err := s.RegisterType("e", AutoIncrement, &anonymous{}); err == nil {
		t.Error("expected anonymous fields to fail")
	}

	if err := s.RegisterType("f", AutoIncrement, &unsupported{}); err == nil {
		t.Error("expected unsupported fields to fail")
	}
}

type user struct {
	Id       int64 `sql:",primary"`
	Name     string
	Age      int64
	Optional *string
}

func TestBuildStruct(t *testing.T) {
	s := NewSchema()
	if err := s.RegisterType("users", AutoIncrement, user{}); err != nil {
		t.Fatal(err)
	}
	table := s.ByName["users"]

	scannables := table.Scannables.Get().([]interface{})
	id := scannables[0].(Scannable)
	name := scannables[1].(Scannable)
	age := scannables[2].(Scannable)
	optional := scannables[3].(Scannable)

	id.Scan(10)
	name.Scan("bob")
	age.Scan(nil)
	optional.Scan(nil)
	if !reflect.DeepEqual(BuildStruct(table, scannables), &user{
		Id:       10,
		Name:     "bob",
		Age:      0,
		Optional: nil,
	}) {
		t.Error("bad build")
	}

	id.Scan(nil)
	name.Scan(nil)
	age.Scan(5)
	optional.Scan("foo")
	var foo = "foo"
	if !reflect.DeepEqual(BuildStruct(table, scannables), &user{
		Id:       0,
		Name:     "",
		Age:      5,
		Optional: &foo,
	}) {
		t.Error("bad build")
	}
}

type customSuffixer struct {
	Valid  bool
	String string
}

func (c *customSuffixer) Scan(value interface{}) error {
	if value == nil {
		c.String = ""
		c.Valid = false
	}
	switch value := value.(type) {
	case nil:
		c.String = ""
		c.Valid = false
	case string:
		c.String = string(value) + "-FOO"
		c.Valid = true
	default:
		return fmt.Errorf("cannot convert %v to string", value)
	}
	return nil
}

func (c *customSuffixer) Value() (driver.Value, error) {
	if !c.Valid {
		return nil, nil
	}
	return c.String, nil
}

type IntAlias int64
type SuffixString string

type custom struct {
	Id           int64 `sql:",primary"`
	IntAlias     IntAlias
	SuffixString SuffixString
}

func TestBuildStructWithAlias(t *testing.T) {
	s := NewSchema()
	s.MustRegisterCustomScalar(SuffixString(""), func() Scannable { return new(customSuffixer) })
	s.MustRegisterSimpleScalar(IntAlias(0))

	if err := s.RegisterType("customs", AutoIncrement, custom{}); err != nil {
		t.Fatal(err)
	}
	table := s.ByName["customs"]

	scannables := table.Scannables.Get().([]interface{})
	id := scannables[0].(Scannable)
	intAlias := scannables[1].(Scannable)
	suffixString := scannables[2].(Scannable)

	id.Scan(10)
	intAlias.Scan(int64(20))
	suffixString.Scan("foo")
	if !reflect.DeepEqual(BuildStruct(table, scannables), &custom{
		Id:           10,
		IntAlias:     20,
		SuffixString: "foo-FOO",
	}) {
		t.Error("bad build")
	}
}

func TestMakeWhere(t *testing.T) {
	s := NewSchema()
	if err := s.RegisterType("users", AutoIncrement, user{}); err != nil {
		t.Fatal(err)
	}
	table := s.ByName["users"]

	where, err := makeWhere(table, Filter{"id": 10})
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(where, &SimpleWhere{
		Columns: []string{"id"},
		Values:  []interface{}{10},
	}) {
		t.Error("bad select")
	}

	where, err = makeWhere(table, Filter{"id": 10, "name": "bob", "age": 30})
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(where, &SimpleWhere{
		Columns: []string{"id", "name", "age"},
		Values:  []interface{}{10, "bob", 30},
	}) {
		t.Error("bad select")
	}

	where, err = makeWhere(table, Filter{})
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(where, &SimpleWhere{
		Columns: []string{},
		Values:  []interface{}{},
	}) {
		t.Error("bad select")
	}

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
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(query, &BaseSelectQuery{
		Table:  s.ByName["users"],
		Filter: Filter{"name": "alice"},
	}) {
		t.Error("bad select")
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
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(query, &InsertQuery{
		Table:   "users",
		Columns: []string{"name", "age", "optional"},
		Values:  []interface{}{"bob", int64(20), nil},
	}) {
		t.Error("bad insert")
	}
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
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(query, &UpsertQuery{
		Table:   "users",
		Columns: []string{"id", "name", "age", "optional"},
		Values:  []interface{}{int64(5), "alice", int64(30), "temp"},
	}) {
		t.Error("bad upsert")
	}
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
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(query, &UpdateQuery{
		Table:   "users",
		Columns: []string{"name", "age", "optional"},
		Values:  []interface{}{"bob", int64(20), nil},
		Where: &SimpleWhere{
			Columns: []string{"id"},
			Values:  []interface{}{int64(10)},
		},
	}) {
		t.Error("bad update")
	}
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
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(query, &UpdateQuery{
		Table:   "users",
		Columns: []string{"name", "age", "optional"},
		Values:  []interface{}{"alice", int64(40), "temp"},
		Where: &SimpleWhere{
			Columns: []string{"id"},
			Values:  []interface{}{int64(20)},
		},
	}) {
		t.Error("bad update")
	}
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
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(query, &DeleteQuery{
		Table: "users",
		Where: &SimpleWhere{
			Columns: []string{"id"},
			Values:  []interface{}{int64(10)},
		},
	}) {
		t.Error("bad delete")
	}
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
