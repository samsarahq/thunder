package schemabuilder

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/samsarahq/thunder/graphql"
)

type alias int64

type root struct {
	X     int64 `graphql:"yyy"`
	Time  time.Time
	Bytes []byte
	Alias alias
}

type schema struct{}

func panicFunction() int64 {
	panic("oh no!")
}

func (s *schema) Query() Object {
	object := Object{
		Type: root{},
	}
	object.FieldFunc("users", func() []*user {
		return []*user{
			{Name: "Alice", Age: 10},
			{Name: "Bob", Age: 20},
		}
	})
	object.FieldFunc("optional", func(args struct{ X *int64 }) int64 {
		if args.X != nil {
			return *args.X
		}
		return -1
	})
	object.FieldFunc("nilObject", func() *user {
		return nil
	})
	object.FieldFunc("nilSlice", func() []*user {
		return nil
	})
	object.FieldFunc("bad", func() (string, error) {
		return "", errors.New("BAD")
	})
	object.FieldFunc("sum", func(args struct{ A, B int64 }) (int64, error) {
		return args.A + args.B, nil
	})
	object.FieldFunc("ints", func() []int64 {
		return []int64{1, 2, 3, 4}
	})
	object.FieldFunc("nested", func(r *root) *root {
		return r
	})
	object.FieldFunc("ptr", func() *user {
		return &user{
			Name: "Charlie",
			Age:  5,
		}
	})
	object.FieldFunc("plain", func() user {
		return user{
			Name: "Jane",
			Age:  5,
		}
	})
	object.FieldFunc("optionalField", func(args struct{ Optional *int64 }) *int64 {
		return args.Optional
	})
	object.FieldFunc("getCtx", func(ctx context.Context) (string, error) {
		return ctx.Value("foo").(string), nil
	})
	object.FieldFunc("panic", func() int64 {
		return panicFunction()
	})

	return object
}

type empty struct{}

func (s *schema) Mutation() Object {
	return Object{
		Type: empty{},
	}
}

type user struct {
	Name string `graphql:",key"`
	Age  int64
}

func (s *schema) User() Object {
	object := Object{
		Type: user{},
	}
	object.FieldFunc("byRef", func(u *user) string {
		return "byRef"
	})
	object.FieldFunc("byVal", func(u user) string {
		return "byVal"
	})
	object.FieldFunc("friends", func(u *user) []*user {
		return []*user{}
	})
	return object
}

func TestExecuteGood(t *testing.T) {
	builtSchema := MustBuildSchema(&schema{})

	r := root{X: 1234, Time: time.Unix(1458757911, 0).UTC(), Bytes: []byte("bar"), Alias: 999}

	ctx := context.WithValue(context.Background(), "foo", "hello there")

	q := graphql.MustParse(`
		{
			users {
				name
				foo: age
				friends { name }
			}
			bar: yyy
			ints
			nested {
				getCtx
				sum(a: 1, b: $var)
			}
			nilObject { name }
			nilSlice { name }
			has: optional(x: 10)
			hasNot: optional
			hasField: optionalField(optional: 10)
			hasNotField: optionalField
			time
			bytes
			ptr { name age byRef byVal }
			plain { name age byRef byVal }
			alias
		}
	`, map[string]interface{}{"var": float64(3)})

	if err := graphql.PrepareQuery(builtSchema.Query, q); err != nil {
		t.Error(err)
	}

	e := graphql.Executor{MaxConcurrency: 1}

	result, err := e.Execute(ctx, builtSchema.Query, r, q)
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(asJSON(result), parseJSON(`
		{"users": [
			{"name": "Alice", "foo": 10, "friends": []},
			{"name": "Bob", "foo": 20, "friends": []}
		],
		"bar": 1234,
		"nilObject": null,
		"nilSlice": [],
		"has": 10,
		"hasNot": -1,
		"hasField": 10,
		"hasNotField": null,
		"ints": [1, 2, 3, 4],
		"nested": {
			"getCtx": "hello there",
			"sum": 4
		},
		"time": "2016-03-23T18:31:51Z",
		"bytes": "YmFy",
		"ptr": {"name": "Charlie", "age": 5, "byRef": "byRef", "byVal": "byVal"},
		"plain": {"name": "Jane", "age": 5, "byRef": "byRef", "byVal": "byVal"},
		"alias": 999}`)) {
		t.Error("bad value")
	}

	if result.(*graphql.DiffableObject).Fields["users"].(*graphql.DiffableList).Items[0].(*graphql.DiffableObject).Key != "Alice" {
		t.Error("expected key")
	}
}

func testMakeGraphql(t *testing.T, s, expected string) {
	actual := makeGraphql(s)
	if actual != expected {
		t.Errorf("makeGraphql(%s) = %s, expected %s", s, actual, expected)
	}
}

func TestMakeGraphql(t *testing.T) {
	testMakeGraphql(t, "FooBar", "fooBar")
	testMakeGraphql(t, "OrganizationId", "organizationId")
	testMakeGraphql(t, "ABC", "aBC")
}

func marshalJSON(v interface{}) string {
	bytes, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(bytes)
}

func parseJSON(s string) interface{} {
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		panic(err)
	}
	return v
}

func asJSON(v interface{}) interface{} {
	return parseJSON(marshalJSON(v))
}

type inner struct {
	Custom float64 `graphql:"foo"`
}

type kitchenSinkArgs struct {
	Child           inner
	Hello           int64
	Hello32         int32
	Hello16         int16
	FooBar          string
	Bool            bool
	OptionalInt     *int64
	OptionalStruct  *inner
	Ints            []int64
	OptionalStructs *[]*inner
	Base64          []byte
	Alias           alias
}

type anonymous struct {
	kitchenSinkArgs
}

type duplicate struct {
	A int64
	B int64 `graphql:"a"`
}

type unsupported struct {
	A byte
}

func testArgParseOk(t *testing.T, p *argParser, input interface{}, expected interface{}) {
	actual, err := p.Parse(input)
	if err != nil {
		t.Error(err)
		return
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("p(%v) = %v, expected %v", input, actual, expected)
	}
}

func testArgParseBad(t *testing.T, p *argParser, input interface{}) {
	if actual, err := p.Parse(input); err == nil {
		t.Errorf("expected p(%v) to fail; got %v", input, actual)
	}
}

func TestArgParser(t *testing.T) {
	parser, _, err := makeArgParser(reflect.TypeOf(kitchenSinkArgs{}))
	if err != nil {
		t.Fatal(err)
	}

	testArgParseOk(t, parser, parseJSON(`
		{
			"child": {"foo": 12.5},
			"hello": 20,
			"hello32": 20,
			"hello16": 20,
			"fooBar": "foo!",
			"bool": true,
			"ints": [1, 2, 3],
			"base64": "Zm9v",
			"alias": 999
		}
	`), kitchenSinkArgs{
		Child:           inner{Custom: 12.5},
		Hello:           20,
		Hello32:         20,
		Hello16:         20,
		FooBar:          "foo!",
		Bool:            true,
		OptionalInt:     nil,
		OptionalStruct:  nil,
		Ints:            []int64{1, 2, 3},
		OptionalStructs: nil,
		Base64:          []byte("foo"),
		Alias:           999,
	})

	var ten = int64(10)

	testArgParseOk(t, parser, parseJSON(`
		{
			"child": {"foo": 22.5},
			"hello": 40,
			"hello32": 40,
			"hello16": 40,
			"fooBar": "bar!",
			"bool": false,
			"optionalInt": 10,
			"optionalStruct": {"foo": 20},
			"ints": [6, 6, 6],
			"optionalStructs": [{"foo": 1}, {"foo": 2}],
			"base64": "MQ==",
			"alias": 1234
		}
	`), kitchenSinkArgs{
		Child:           inner{Custom: 22.5},
		Hello:           40,
		Hello32:         40,
		Hello16:         40,
		FooBar:          "bar!",
		Bool:            false,
		OptionalInt:     &ten,
		OptionalStruct:  &inner{Custom: 20},
		Ints:            []int64{6, 6, 6},
		OptionalStructs: &[]*inner{{Custom: 1}, {Custom: 2}},
		Base64:          []byte("1"),
		Alias:           1234,
	})

	testArgParseBad(t, parser, parseJSON(`
		{
			"child": {"bar": 22.5},
			"hello": 40,
			"fooBar": "bar!",
			"bool": false,
			"ints": [1, 2, 3],
			"base64": "Zm9v",
			"alias": 999
		}
	`))

	testArgParseBad(t, parser, parseJSON(`
		{
			"child": {"foo": 22.5},
			"hello": "xyz",
			"fooBar": "bar!",
			"bool": false,
			"ints": [1, 2, 3],
			"base64": "Zm9v",
			"alias": 999
		}
	`))

	testArgParseBad(t, parser, parseJSON(`
		{
			"child": {"foo": 22.5},
			"hello": 40,
			"fooBar": {"xyz": "abc"},
			"bool": false,
			"ints": [1, 2, 3],
			"base64": "Zm9v",
			"alias": 999
		}
	`))

	testArgParseBad(t, parser, parseJSON(`
		{
			"child": {"foo": 22.5},
			"hello": 40,
			"fooBar": {"xyz": "abc"},
			"bool": false,
			"ints": [1, 2, "foo"],
			"base64": "Zm9v",
			"alias": 999
		}
	`))

	testArgParseBad(t, parser, parseJSON(`
		{
			"child": {"foo": 12.5},
			"hello": 20,
			"fooBar": "foo!",
			"bool": true,
			"ints": [1, 2, 3],
			"base64": "a",
			"alias": 999
		}
	`))

	if _, _, err := makeArgParser(reflect.TypeOf(&duplicate{})); err == nil {
		t.Error("expected duplicate fields to fail")
	}

	if _, _, err := makeArgParser(reflect.TypeOf(&anonymous{})); err == nil {
		t.Error("expected anonymous fields to fail")
	}

	if _, _, err := makeArgParser(reflect.TypeOf(&unsupported{})); err == nil {
		t.Error("expected unsupported fields to fail")
	}
}
