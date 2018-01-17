package schemabuilder

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/internal"
)

type alias int64

type Root struct {
	X     int64 `graphql:"yyy"`
	Time  time.Time
	Bytes []byte
	Alias alias
}

type User struct {
	Name string `graphql:",key"`
	Age  int
}

type WeirdKey struct {
}

func panicFunction() int64 {
	panic("oh no!")
}

func TestExecuteGood(t *testing.T) {
	schema := NewSchema()

	query := schema.Query()
	query.FieldFunc("users", func() []*User {
		return []*User{
			{Name: "Alice", Age: 10},
			{Name: "Bob", Age: 20},
		}
	})
	query.FieldFunc("optional", func(args struct{ X *int64 }) int64 {
		if args.X != nil {
			return *args.X
		}
		return -1
	})
	query.FieldFunc("nilObject", func() *User {
		return nil
	})
	query.FieldFunc("requiredObject", func() *User {
		return &User{}
	}, NonNullable)
	query.FieldFunc("nilSlice", func() []*User {
		return nil
	})
	query.FieldFunc("bad", func() (string, error) {
		return "", errors.New("BAD")
	})
	query.FieldFunc("sum", func(args struct{ A, B int64 }) (int64, error) {
		return args.A + args.B, nil
	})
	query.FieldFunc("ints", func() []int64 {
		return []int64{1, 2, 3, 4}
	})
	query.FieldFunc("ptr", func() *User {
		return &User{
			Name: "Charlie",
			Age:  5,
		}
	})
	query.FieldFunc("plain", func() User {
		return User{
			Name: "Jane",
			Age:  5,
		}
	})
	query.FieldFunc("optionalField", func(args struct{ Optional *int64 }) *int64 {
		return args.Optional
	})
	query.FieldFunc("getCtx", func(ctx context.Context) (string, error) {
		return ctx.Value("foo").(string), nil
	})
	query.FieldFunc("panic", func() int64 {
		return panicFunction()
	})
	query.FieldFunc("root", func() Root {
		return Root{X: 1234, Time: time.Unix(1458757911, 0).UTC(), Bytes: []byte("bar"), Alias: 999}
	})

	query.FieldFunc("weirdKey", func() WeirdKey {
		return WeirdKey{}
	})

	_ = schema.Mutation()

	root := schema.Object("root", Root{})
	root.FieldFunc("nested", func(r *Root) *Root {
		return r
	})

	user := schema.Object("User", User{})
	user.FieldFunc("byRef", func(u *User) string {
		return "byRef"
	})
	user.FieldFunc("byVal", func(u User) string {
		return "byVal"
	})
	user.FieldFunc("friends", func(u *User) []*User {
		return []*User{}
	})

	extendUser := schema.Object("User", User{})
	extendUser.FieldFunc("extended", func(u User) string {
		return "extended"
	})

	weirdKey := schema.Object("weirdKey", WeirdKey{})
	weirdKey.Key("key")
	weirdKey.FieldFunc("key", func(w WeirdKey) int64 {
		return -1
	})

	builtSchema := schema.MustBuild()

	ctx := context.WithValue(context.Background(), "foo", "hello there")

	q := graphql.MustParse(`
		{
			users {
				name
				foo: age
				friends { name }
				extended
			}
			ints
			getCtx
			sum(a: 1, b: $var)
			nilObject { name }
			nilSlice { name }
			has: optional(x: 10)
			hasNot: optional
			hasField: optionalField(optional: 10)
			hasNotField: optionalField
			ptr { name age byRef byVal }
			plain { name age byRef byVal }
			root { nested { time bar: yyy bytes alias } }
			weirdKey { key }
		}
	`, map[string]interface{}{"var": float64(3)})

	if err := graphql.PrepareQuery(builtSchema.Query, q.SelectionSet); err != nil {
		t.Error(err)
	}

	e := graphql.Executor{}

	result, err := e.Execute(ctx, builtSchema.Query, nil, q)
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(internal.AsJSON(result), internal.ParseJSON(`
		{"users": [
			{"name": "Alice", "foo": 10, "friends": [], "extended": "extended", "__key": "Alice"},
			{"name": "Bob", "foo": 20, "friends": [], "extended": "extended", "__key": "Bob"}
		],
		"nilObject": null,
		"nilSlice": [],
		"has": 10,
		"hasNot": -1,
		"hasField": 10,
		"hasNotField": null,
		"ints": [1, 2, 3, 4],
		"getCtx": "hello there",
		"sum": 4,
		"ptr": {"name": "Charlie", "age": 5, "byRef": "byRef", "byVal": "byVal", "__key": "Charlie"},
		"plain": {"name": "Jane", "age": 5, "byRef": "byRef", "byVal": "byVal", "__key": "Jane"},
		"root": {"nested": {"time": "2016-03-23T18:31:51Z", "bytes": "YmFy", "bar": 1234, "alias": 999}},
		"weirdKey": {"key": -1, "__key": -1}
		}`)) {
		t.Error("bad value")
	}
}

func TestExecuteErrorNullReturn(t *testing.T) {
	schema := NewSchema()
	query := schema.Query()
	query.FieldFunc("required", func() *int64 {
		return nil
	}, NonNullable)

	builtSchema := schema.MustBuild()

	q := graphql.MustParse(`
		{
			required
		}
	`, map[string]interface{}{"var": float64(3)})

	if err := graphql.PrepareQuery(builtSchema.Query, q.SelectionSet); err != nil {
		t.Error(err)
	}

	e := graphql.Executor{}
	_, err := e.Execute(context.Background(), builtSchema.Query, nil, q)
	if err == nil {
		t.Error("expected error, but received nil")
	}

	if !strings.Contains(err.Error(), "is marked non-nullable but returned a null value") {
		t.Errorf("expected error for null return, but received %s", err.Error())
	}
}

func TestExecuteErrorBasic(t *testing.T) {
	schema := NewSchema()
	query := schema.Query()
	query.FieldFunc("field", func() (*int64, error) {
		return nil, errors.New("an error occurred during computation")
	}, NonNullable)

	builtSchema := schema.MustBuild()

	q := graphql.MustParse(`
		{
			field
		}
	`, map[string]interface{}{"var": float64(3)})

	if err := graphql.PrepareQuery(builtSchema.Query, q.SelectionSet); err != nil {
		t.Error(err)
	}

	e := graphql.Executor{}
	_, err := e.Execute(context.Background(), builtSchema.Query, nil, q)
	if err == nil {
		t.Error("expected error, but received nil")
	}

	if !strings.Contains(err.Error(), "an error occurred during computation") {
		t.Errorf("expected resolver error, but received %s", err.Error())
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

type inner struct {
	Custom float64 `graphql:"foo"`
}

type structAlias inner

type kitchenSinkArgs struct {
	Child               inner
	Hello               int64
	Hello32             int32
	Hello16             int16
	FooBar              string
	Bool                bool
	OptionalInt         *int64
	OptionalStruct      *inner
	Ints                []int64
	OptionalStructs     *[]*inner
	Base64              []byte
	Alias               alias
	OptionalAlias       *alias
	StructAlias         structAlias
	OptionalStructAlias *structAlias
	Time                time.Time
	OptionalTime        *time.Time
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

	testArgParseOk(t, parser, internal.ParseJSON(`
		{
			"child": {"foo": 12.5},
			"hello": 20,
			"hello32": 20,
			"hello16": 20,
			"fooBar": "foo!",
			"bool": true,
			"ints": [1, 2, 3],
			"base64": "Zm9v",
			"alias": 999,
			"structAlias": {"foo": 14},
			"time": "2016-08-31T00:00:00Z"
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
		OptionalAlias:   nil,
		StructAlias:     structAlias{Custom: 14},
		Time:            time.Unix(1472601600, 0).UTC(),
	})

	var ten = int64(10)
	var aliasTen = alias(10)
	var day = time.Unix(1504137600, 0).UTC()

	testArgParseOk(t, parser, internal.ParseJSON(`
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
			"alias": 1234,
			"optionalAlias": 10,
			"structAlias": {"foo": 14},
			"optionalStructAlias": {"foo": 17},
			"time": "2016-08-31T00:00:00Z",
			"optionalTime": "2017-08-31T00:00:00Z"
		}
	`), kitchenSinkArgs{
		Child:               inner{Custom: 22.5},
		Hello:               40,
		Hello32:             40,
		Hello16:             40,
		FooBar:              "bar!",
		Bool:                false,
		OptionalInt:         &ten,
		OptionalStruct:      &inner{Custom: 20},
		Ints:                []int64{6, 6, 6},
		OptionalStructs:     &[]*inner{{Custom: 1}, {Custom: 2}},
		Base64:              []byte("1"),
		Alias:               1234,
		OptionalAlias:       &aliasTen,
		StructAlias:         structAlias{Custom: 14},
		OptionalStructAlias: &structAlias{Custom: 17},
		Time:                time.Unix(1472601600, 0).UTC(),
		OptionalTime:        &day,
	})

	testArgParseBad(t, parser, internal.ParseJSON(`
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

	testArgParseBad(t, parser, internal.ParseJSON(`
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

	testArgParseBad(t, parser, internal.ParseJSON(`
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

	testArgParseBad(t, parser, internal.ParseJSON(`
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

	testArgParseBad(t, parser, internal.ParseJSON(`
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
