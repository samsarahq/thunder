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

func parseJSON(s string) interface{} {
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		panic(err)
	}
	return v
}

type inner struct {
	Custom float64 `graphql:"foo"`
}

type alias int64

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
	parser, err := makeArgParser(reflect.TypeOf(kitchenSinkArgs{}))
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

	if _, err := makeArgParser(reflect.TypeOf(&duplicate{})); err == nil {
		t.Error("expected duplicate fields to fail")
	}

	if _, err := makeArgParser(reflect.TypeOf(&anonymous{})); err == nil {
		t.Error("expected anonymous fields to fail")
	}

	if _, err := makeArgParser(reflect.TypeOf(&unsupported{})); err == nil {
		t.Error("expected unsupported fields to fail")
	}
}

type user struct {
	Name string `graphql:",key"`
	Age  int64
}

type auto struct {
	Auto string
}

type empty struct{}

type root struct {
	Foo      int64
	Bar      string
	Baz      []int64 `graphql:"array"`
	Optional *int64
	Time     time.Time
	Bytes    []byte
	Plain    user
	Alias    alias
}

type schema struct {
}

func (s *schema) Query() Spec {
	spec := Spec{
		Type: root{},
	}
	spec.FieldFunc("auto", func() auto {
		return auto{Auto: "automagic"}
	})
	spec.FieldFunc("alice", func(r *root) *user {
		return &user{
			Name: "alice",
			Age:  10,
		}
	})
	spec.FieldFunc("sum", func(r *root, args struct {
		A, B int64
	}) (int64, error) {
		return args.A + args.B, nil
	})
	spec.FieldFunc("argsAndCtx", func(r *root, args struct {
		A, B int64
	}) (int64, error) {
		return args.A + args.B, nil
	})
	spec.FieldFunc("argsCtxAndSelectionSet", func(r *root, args struct{ A, B int64 }, s *graphql.SelectionSet) (int64, error) {
		return int64(len(s.Selections)), nil
	})
	spec.FieldFunc("ctxAndSelectionSet", func(r *root, s *graphql.SelectionSet) (int64, error) {
		return int64(len(s.Selections)), nil
	})
	spec.FieldFunc("noSource", func(ctx context.Context, args struct{ A, B int64 }) int64 {
		return args.A + args.B*2
	})
	spec.FieldFunc("bad", func(r *root) (bool, error) {
		return false, errors.New("bad")
	})
	spec.FieldFunc("justError", func(r *root, args struct{ Ok bool }) error {
		if args.Ok {
			return nil
		} else {
			return errors.New("bad")
		}
	})
	return spec
}

func (s *schema) Mutation() Spec {
	return Spec{
		Type: empty{},
	}
}

func (s *schema) User() Spec {
	spec := Spec{
		Type: user{},
	}
	spec.FieldFunc("ctx", func(ctx context.Context, u *user) bool {
		return ctx.Value("flag").(bool)
	})
	return spec
}

func TestMakeSchema(t *testing.T) {
	schema := MustBuildSchema(&schema{})
	obj := schema.Query.(*graphql.Object)

	if obj.Name != "root" {
		t.Errorf("bad name '%s'", obj.Name)
	}

	r := root{Foo: 10, Bar: "abc", Baz: []int64{1, 2, 3}, Optional: nil, Time: time.Unix(1458757911, 0).UTC(), Bytes: []byte("foo"), Alias: 1234}

	if ret, err := obj.Fields["sum"].Resolve(context.Background(), r, struct{ A, B int64 }{A: 5, B: 5}, nil); err != nil || ret != int64(10) {
		t.Error("bad sum")
	}

	if ret, err := obj.Fields["noSource"].Resolve(context.Background(), r, struct{ A, B int64 }{A: 3, B: 8}, nil); err != nil || ret != int64(19) {
		t.Error("bad noSource")
	}

	alice, err := obj.Fields["alice"].Resolve(context.Background(), r, nil, nil)
	if err != nil || !reflect.DeepEqual(alice, &user{Name: "alice", Age: 10}) {
		t.Error("bad alice")
	}

	if ret, err := obj.Fields["ctxAndSelectionSet"].Resolve(context.Background(), r, nil, &graphql.SelectionSet{Selections: []*graphql.Selection{nil}}); err != nil || ret != int64(1) {
		t.Error("bad selection set")
	}

	if ctx, err := obj.Fields["alice"].Type.(*graphql.Object).Fields["ctx"].Resolve(context.WithValue(context.Background(), "flag", true), alice, nil, nil); err != nil || ctx != true {
		t.Error("bad ctx")
	}

	if key, err := obj.Fields["alice"].Type.(*graphql.Object).Key(context.Background(), alice, nil, nil); err != nil || key != "alice" {
		t.Error("bad key")
	}

	if res, err := obj.Fields["bad"].Resolve(context.Background(), r, nil, nil); res != nil || err == nil {
		t.Error("bad bad")
	}

	if res, err := obj.Fields["justError"].Resolve(context.Background(), r, struct{ Ok bool }{true}, nil); res != true || err != nil {
		t.Error("bad ok justError")
	}

	if res, err := obj.Fields["justError"].Resolve(context.Background(), r, struct{ Ok bool }{false}, nil); res != nil || err == nil {
		t.Error("bad bad justError")
	}

	if foo, err := obj.Fields["foo"].Resolve(context.Background(), r, nil, nil); err != nil || foo != int64(10) {
		t.Error("bad foo")
	}

	if res, err := obj.Fields["auto"].Resolve(context.Background(), r, nil, nil); err != nil || !reflect.DeepEqual(res, auto{Auto: "automagic"}) {
		t.Error("bad auto")
	}

	if res, err := obj.Fields["auto"].Type.(*graphql.Object).Fields["auto"].Resolve(context.Background(), auto{Auto: "automagic"}, nil, nil); err != nil || res != "automagic" {
		t.Error("bad auto")
	}

	if optional, err := obj.Fields["optional"].Resolve(context.Background(), r, nil, nil); err != nil || optional != (*int64)(nil) {
		t.Error("bad optional")
	}

	if array, err := obj.Fields["array"].Resolve(context.Background(), r, nil, nil); err != nil || !reflect.DeepEqual(array, []int64{1, 2, 3}) {
		t.Error("bad array")
	}

	if ts, err := obj.Fields["time"].Resolve(context.Background(), r, nil, nil); err != nil || !ts.(time.Time).Equal(time.Unix(1458757911, 0).UTC()) {
		t.Error("bad time")
	}

	if bytes, err := obj.Fields["bytes"].Resolve(context.Background(), r, nil, nil); err != nil || !reflect.DeepEqual(bytes, []byte("foo")) {
		t.Error("bad bytes")
	}

	if plain, err := obj.Fields["plain"].Resolve(context.Background(), r, nil, nil); err != nil || !reflect.DeepEqual(plain, user{}) {
		t.Error("bad plain")
	}

	if a, err := obj.Fields["alias"].Resolve(context.Background(), r, nil, nil); err != nil || a != alias(1234) {
		t.Error("bad alias")
	}

	if name, err := obj.Fields["plain"].Type.(*graphql.Object).Fields["name"].Resolve(context.Background(), user{Name: "foo"}, nil, nil); err != nil || !reflect.DeepEqual(name, "foo") {
		t.Error("bad name")
	}

	/*
		if _, err := BuildSchema(&badType{}, &empty{}, []Spec{badTypeSpec, emptySpec}); err == nil {
			t.Error("expected bad type to fail")
		}

		if _, err := BuildSchema(&badRet{}, &empty{}, []Spec{badRetSpec, emptySpec}); err == nil {
			t.Error("expected bad ret to fail")
		}

		if _, err := BuildSchema(&badArgs{}, &empty{}, []Spec{badArgsSpec, emptySpec}); err == nil {
			t.Error("expected bad args to fail")
		}
	*/
}

/*
type badType struct {
	// map not allowed
	Baz map[string]int
}

var badTypeSpec = Spec{
	Type:    badType{},
	Methods: nil,
}

type badRet struct {
}

var badRetSpec = Spec{
	Type: badType{},
	Methods: Methods{
		// map not allowed
		"foo": func(r *badRet) map[string]int {
			return nil
		},
	},
}

type badArgs struct {
}

var badArgsSpec = Spec{
	Type: badType{},
	Methods: Methods{
		// args must be wrapped in struct
		"foo": func(r *badArgs, x int) int {
			return 0
		},
	},
}
*/
