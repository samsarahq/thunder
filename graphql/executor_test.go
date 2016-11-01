package graphql_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
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

func (s *schema) Query() schemabuilder.Spec {
	spec := schemabuilder.Spec{
		Type: root{},
	}
	spec.FieldFunc("users", func() []*user {
		return []*user{
			{Name: "Alice", Age: 10},
			{Name: "Bob", Age: 20},
		}
	})
	spec.FieldFunc("optional", func(args struct{ X *int64 }) int64 {
		if args.X != nil {
			return *args.X
		}
		return -1
	})
	spec.FieldFunc("nilObject", func() *user {
		return nil
	})
	spec.FieldFunc("nilSlice", func() []*user {
		return nil
	})
	spec.FieldFunc("bad", func() (string, error) {
		return "", errors.New("BAD")
	})
	spec.FieldFunc("sum", func(args struct{ A, B int64 }) (int64, error) {
		return args.A + args.B, nil
	})
	spec.FieldFunc("ints", func() []int64 {
		return []int64{1, 2, 3, 4}
	})
	spec.FieldFunc("nested", func(r *root) *root {
		return r
	})
	spec.FieldFunc("ptr", func() *user {
		return &user{
			Name: "Charlie",
			Age:  5,
		}
	})
	spec.FieldFunc("plain", func() user {
		return user{
			Name: "Jane",
			Age:  5,
		}
	})
	spec.FieldFunc("optionalField", func(args struct{ Optional *int64 }) *int64 {
		return args.Optional
	})
	spec.FieldFunc("getCtx", func(ctx context.Context) (string, error) {
		return ctx.Value("foo").(string), nil
	})
	spec.FieldFunc("panic", func() int64 {
		return panicFunction()
	})

	return spec
}

type empty struct{}

func (s *schema) Mutation() schemabuilder.Spec {
	return schemabuilder.Spec{
		Type: empty{},
	}
}

type user struct {
	Name string `graphql:",key"`
	Age  int64
}

func (s *schema) User() schemabuilder.Spec {
	spec := schemabuilder.Spec{
		Type: user{},
	}
	spec.FieldFunc("byRef", func(u *user) string {
		return "byRef"
	})
	spec.FieldFunc("byVal", func(u user) string {
		return "byVal"
	})
	spec.FieldFunc("friends", func(u *user) []*user {
		return []*user{}
	})
	return spec
}

var builtSchema = schemabuilder.MustBuildSchema(&schema{})

func TestExecuteGood(t *testing.T) {
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

	if err := graphql.PrepareQuery(builtSchema.QueryType, q); err != nil {
		t.Error(err)
	}

	e := graphql.Executor{MaxConcurrency: 1}

	result, err := e.Execute(ctx, builtSchema.QueryType, r, q)
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

	// TODO: Verify caching and concurrency
}

func TestBad(t *testing.T) {
	r := &root{X: 1234}

	q := graphql.MustParse(`
    {
      bad
    }
  `, map[string]interface{}{})

	if err := graphql.PrepareQuery(builtSchema.QueryType, q); err != nil {
		t.Error(err)
	}

	e := graphql.Executor{MaxConcurrency: 1}
	_, err := e.Execute(context.Background(), builtSchema.QueryType, r, q)
	if err == nil {
		t.Error("expected bad error")
	}
}

func TestMissingField(t *testing.T) {
	q := graphql.MustParse(`
    {
      unknown
    }
  `, map[string]interface{}{})

	if err := graphql.PrepareQuery(builtSchema.QueryType, q); err == nil {
		t.Error("expected error")
	}
}

func TestMissingSelectors(t *testing.T) {
	q := graphql.MustParse(`
    {
      nested
    }
  `, map[string]interface{}{})

	if err := graphql.PrepareQuery(builtSchema.QueryType, q); err == nil {
		t.Error("expected error")
	}
}

func TestUnwantedSelectors(t *testing.T) {
	q := graphql.MustParse(`
    {
      bar { bar }
    }
  `, map[string]interface{}{})

	if err := graphql.PrepareQuery(builtSchema.QueryType, q); err == nil {
		t.Error("expected error")
	}
}

func TestBadArgs(t *testing.T) {
	q := graphql.MustParse(`
    {
      sum(a: "123", b: 4)
    }
  `, map[string]interface{}{})

	if err := graphql.PrepareQuery(builtSchema.QueryType, q); err == nil {
		t.Error("expected error")
	}
}

// TestReportPanic tests that a panicing resolver will report an error to a
// context implementing PanicReporter instead of crashing the server.
func TestReportPanic(t *testing.T) {
	q := graphql.MustParse(`
    {
			panic
		}
  `, nil)

	if err := graphql.PrepareQuery(builtSchema.QueryType, q); err != nil {
		t.Error(err)
	}

	e := graphql.Executor{MaxConcurrency: 1}

	_, err := e.Execute(context.Background(), builtSchema.QueryType, root{}, q)
	if err == nil {
		t.Error("expected error from panic")
	}
	if !strings.Contains(err.Error(), "oh no!") {
		t.Error("expected panic to be caught")
	}
	if !strings.Contains(err.Error(), "panicFunction") {
		t.Error("expected panic to contain stacktrace")
	}
}
