package graphql

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/samsarahq/thunder/internal"
)

func makeQuery(onArgParse *func()) *Object {
	noArguments := func(json interface{}) (interface{}, error) {
		return nil, nil
	}

	query := &Object{
		Name:   "Query",
		Fields: make(map[string]*Field),
	}

	a := &Object{
		Name: "A",
		KeyField: &Field{
			Resolve: func(ctx context.Context, source, args interface{}, selectionSet *SelectionSet) (interface{}, error) {
				return source, nil
			},
			Type: &Scalar{Type: "string"},
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

	a.Fields["valuePtr"] = &Field{
		Resolve: func(ctx context.Context, source, args interface{}, selectionSet *SelectionSet) (interface{}, error) {
			temp := source.(int)
			if temp%2 == 0 {
				return nil, nil
			}
			return &temp, nil
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

	a.Fields["fieldWithArgs"] = &Field{
		Resolve: func(ctx context.Context, source, args interface{}, selectionSet *SelectionSet) (interface{}, error) {
			return 1, nil
		},
		Type: &Scalar{Type: "int"},
		ParseArguments: func(json interface{}) (interface{}, error) {
			if onArgParse != nil {
				(*onArgParse)()
			}
			return nil, nil
		},
	}

	return query
}

func TestBasic(t *testing.T) {
	query := makeQuery(nil)

	q := MustParse(`{
		static
		a { value nested { value } }
		as { value valuePtr }
	}`, nil)

	if err := PrepareQuery(query, q.SelectionSet); err != nil {
		t.Error(err)
	}
	e := Executor{}
	result, err := e.Execute(context.Background(), query, nil, q)
	if err != nil {
		t.Error(err)
	}

	// assert that result["as"][1]["valuePtr"] == 1 (and not a pointer to 1)
	root, _ := internal.AsJSON(result).(map[string]interface{})
	as, _ := root["as"].([]interface{})
	asObject, _ := as[1].(map[string]interface{})
	if int(asObject["valuePtr"].(float64)) != 1 {
		t.Error("Expected valuePtr to be 1, was", asObject["valuePtr"])
	}

	if !reflect.DeepEqual(internal.AsJSON(result), internal.ParseJSON(`
		{
			"static": "static",
			"a": {
				"value": 0,
				"__key": 0,
				"nested": {
					"value": 1,
					"__key": 1
				}
			},
			"as": [
			        {"value": 0, "valuePtr": null, "__key": 0},
				{"value": 1, "valuePtr": 1, "__key": 1},
				{"value": 2, "valuePtr": null, "__key": 2},
				{"value": 3, "valuePtr": 3, "__key": 3}
			]
		}`)) {
		t.Error("bad value", spew.Sdump(internal.AsJSON(result)))
	}
}

func TestRepeatedFragment(t *testing.T) {
	ctr := 0
	countArgParse := func() {
		ctr++
	}
	query := makeQuery(&countArgParse)

	q := MustParse(`{
		static
		a { value nested { value ...frag } ...frag }
		as { value }
	}
	fragment frag on A {
		fieldWithArgs(arg1: 1)
	}
	`, nil)

	if err := PrepareQuery(query, q.SelectionSet); err != nil {
		t.Error(err)
	}
	e := Executor{}
	_, err := e.Execute(context.Background(), query, nil, q)
	if err != nil {
		t.Error(err)
	}

	if ctr != 1 {
		t.Errorf("Expected args for fragment to be parsed once, but they were parsed %d times.", ctr)
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
	query := makeQuery(nil)

	q := MustParse(`
		query foo {
			error
		}
	`, map[string]interface{}{})

	if err := PrepareQuery(query, q.SelectionSet); err != nil {
		t.Error(err)
	}

	e := Executor{}
	_, err := e.Execute(context.Background(), query, nil, q)
	if err == nil || err.Error() != "foo.error: test error" {
		t.Error("expected test error")
	}
}

// TestPanic tests that a panicing resolver will report an error to a
// context implementing PanicReporter instead of crashing the server.
func TestPanic(t *testing.T) {
	query := makeQuery(nil)

	q := MustParse(`
		{
			panic
		}
	`, nil)

	if err := PrepareQuery(query, q.SelectionSet); err != nil {
		t.Error(err)
	}

	e := Executor{}

	_, err := e.Execute(context.Background(), query, nil, q)
	if err == nil || !strings.Contains(err.Error(), "test panic") {
		t.Error("expected test panic")
	}
	if !strings.Contains(err.Error(), "executor_test.go") {
		t.Error("expected stacktrace")
	}
}

// TODO: Verify caching and concurrency
