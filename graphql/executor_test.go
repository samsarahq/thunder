package graphql

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

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

func makeQuery() *Object {
	noArguments := func(json interface{}) (interface{}, error) {
		return nil, nil
	}

	query := &Object{
		Name:   "Query",
		Fields: make(map[string]*Field),
	}

	a := &Object{
		Name: "A",
		Key: func(ctx context.Context, source, args interface{}, selectionSet *SelectionSet) (interface{}, error) {
			return source, nil
		},
		Fields: make(map[string]*Field),
	}

	query.Fields["a"] = &Field{
		Resolve: func(ctx context.Context, source, args interface{}, selectionSet *SelectionSet) (interface{}, error) {
			return 0, nil
		},
		Type:           a,
		ParseArguments: noArguments,
	}

	query.Fields["as"] = &Field{
		Resolve: func(ctx context.Context, source, args interface{}, selectionSet *SelectionSet) (interface{}, error) {
			return []int{0, 1, 2, 3}, nil
		},
		Type:           &List{Type: a},
		ParseArguments: noArguments,
	}

	query.Fields["static"] = &Field{
		Resolve: func(ctx context.Context, source, args interface{}, selectionSet *SelectionSet) (interface{}, error) {
			return "static", nil
		},
		Type:           &Scalar{Type: "string"},
		ParseArguments: noArguments,
	}

	query.Fields["error"] = &Field{
		Resolve: func(ctx context.Context, source, args interface{}, selectionSet *SelectionSet) (interface{}, error) {
			return nil, errors.New("test error")
		},
		Type:           &Scalar{Type: "string"},
		ParseArguments: noArguments,
	}

	query.Fields["panic"] = &Field{
		Resolve: func(ctx context.Context, source, args interface{}, selectionSet *SelectionSet) (interface{}, error) {
			panic("test panic")
		},
		Type:           &Scalar{Type: "string"},
		ParseArguments: noArguments,
	}

	a.Fields["value"] = &Field{
		Resolve: func(ctx context.Context, source, args interface{}, selectionSet *SelectionSet) (interface{}, error) {
			return source.(int), nil
		},
		Type:           &Scalar{Type: "int"},
		ParseArguments: noArguments,
	}

	a.Fields["nested"] = &Field{
		Resolve: func(ctx context.Context, source, args interface{}, selectionSet *SelectionSet) (interface{}, error) {
			return source.(int) + 1, nil
		},
		Type:           a,
		ParseArguments: noArguments,
	}

	return query
}

func TestBasic(t *testing.T) {
	query := makeQuery()

	q := MustParse(`{
		static
		a { value nested { value } }
		as { value }
	}`, nil)

	if err := PrepareQuery(query, q); err != nil {
		t.Error(err)
	}
	e := Executor{MaxConcurrency: 1}
	result, err := e.Execute(context.Background(), query, nil, q)
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(asJSON(result), parseJSON(`
		{
			"static": "static",
			"a": {
				"value": 0,
				"nested": {
					"value": 1
				}
			},
			"as": [
				{"value": 0},
				{"value": 1},
				{"value": 2},
				{"value": 3}
			]
		}`)) {
		t.Error("bad value", spew.Sdump(asJSON(result)))
	}
}

/*
func TestMissingField(t *testing.T) {
	q := MustParse(`
		{
			unknown
		}
	`, map[string]interface{}{})

	if err := PrepareQuery(query, q); err == nil {
		t.Error("expected error")
	}
}

func TestMissingSelectors(t *testing.T) {
	q := MustParse(`
		{
			nested
		}
	`, map[string]interface{}{})

	if err := PrepareQuery(query, q); err == nil {
		t.Error("expected error")
	}
}

func TestUnwantedSelectors(t *testing.T) {
	q := MustParse(`
		{
			bar { bar }
		}
	`, map[string]interface{}{})

	if err := PrepareQuery(query, q); err == nil {
		t.Error("expected error")
	}
}

func TestBadArgs(t *testing.T) {
	q := MustParse(`
		{
			sum(a: "123", b: 4)
		}
	`, map[string]interface{}{})

	if err := PrepareQuery(query, q); err == nil {
		t.Error("expected error")
	}
}
*/

func TestError(t *testing.T) {
	query := makeQuery()

	q := MustParse(`
		{
			error
		}
	`, map[string]interface{}{})

	if err := PrepareQuery(query, q); err != nil {
		t.Error(err)
	}

	e := Executor{MaxConcurrency: 1}
	_, err := e.Execute(context.Background(), query, nil, q)
	if err == nil || !strings.Contains(err.Error(), "test error") {
		t.Error("expected test error")
	}
}

// TestPanic tests that a panicing resolver will report an error to a
// context implementing PanicReporter instead of crashing the server.
func TestPanic(t *testing.T) {
	query := makeQuery()

	q := MustParse(`
		{
			panic
		}
	`, nil)

	if err := PrepareQuery(query, q); err != nil {
		t.Error(err)
	}

	e := Executor{MaxConcurrency: 1}

	_, err := e.Execute(context.Background(), query, nil, q)
	if err == nil || !strings.Contains(err.Error(), "test panic") {
		t.Error("expected test panic")
	}
	if !strings.Contains(err.Error(), "executor_test.go") {
		t.Error("expected stacktrace")
	}
}

// TODO: Verify caching and concurrency
