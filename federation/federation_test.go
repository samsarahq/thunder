package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/samsarahq/thunder/batch"
	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
)

func makeExecutors(schemas map[string]*schemabuilder.Schema) (map[string]ExecutorClient, error) {
	executors := make(map[string]ExecutorClient)

	for name, schema := range schemas {
		srv, err := NewServer(schema.MustBuild())
		if err != nil {
			return nil, err
		}

		executors[name] = &DirectExecutorClient{Client: srv}
	}

	return executors, nil
}

type Enum int

type Foo struct {
	Name string
}

type Bar struct {
	Id int64
}

type FooOrBar struct {
	schemabuilder.Union
	*Foo
	*Bar
}

type Pair struct {
	A, B int64
}

func buildTestSchema1() *schemabuilder.Schema {
	schema := schemabuilder.NewSchema()

	query := schema.Query()
	query.FieldFunc("s1f", func() *Foo {
		return &Foo{
			Name: "jimbob",
		}
	})
	query.FieldFunc("s1fff", func() []*Foo {
		return []*Foo{
			{
				Name: "jimbo",
			},
			{
				Name: "bob",
			},
		}
	})

	query.FieldFunc("s1echo", func(args struct {
		Foo      string
		Required Pair
		Optional *int64
	}) string {
		return fmt.Sprintf("%s %v %v", args.Foo, args.Required, args.Optional)
	})

	schema.Enum(Enum(1), map[string]Enum{
		"one": 1,
	})

	mutation := schema.Mutation()

	mutation.FieldFunc("s1addFoo", func(args struct{ Name string }) *Foo {
		return &Foo{
			Name: args.Name,
		}
	})

	foo := schema.Object("Foo", Foo{})
	foo.Federation(func(f *Foo) string {
		return f.Name
	})
	foo.BatchFieldFunc("s1hmm", func(ctx context.Context, in map[batch.Index]*Foo) (map[batch.Index]string, error) {
		out := make(map[batch.Index]string)
		for i, foo := range in {
			out[i] = foo.Name + "!!!"
		}
		return out, nil
	})
	foo.FieldFunc("s1nest", func(f *Foo) *Foo {
		return f
	})
	foo.FieldFunc("s1enum", func(f *Foo) Enum {
		return Enum(1)
	})

	schema.Federation().FieldFunc("Bar", func(args struct{ Keys []int64 }) []*Bar {
		bars := make([]*Bar, 0, len(args.Keys))
		for _, key := range args.Keys {
			bars = append(bars, &Bar{Id: key})
		}
		return bars
	})

	bar := schema.Object("Bar", Bar{})
	bar.FieldFunc("s1baz", func(b *Bar) string {
		return fmt.Sprint(b.Id)
	})

	query.FieldFunc("s1both", func() []FooOrBar {
		return []FooOrBar{
			{
				Foo: &Foo{
					Name: "this is the foo",
				},
			},
			{
				Bar: &Bar{
					Id: 1234,
				},
			},
		}
	})

	return schema
}

func buildTestSchema2() *schemabuilder.Schema {
	schema := schemabuilder.NewSchema()

	schema.Federation().FieldFunc("Foo", func(args struct{ Keys []string }) []*Foo {
		foos := make([]*Foo, 0, len(args.Keys))
		for _, key := range args.Keys {
			foos = append(foos, &Foo{Name: key})
		}
		return foos
	})

	schema.Query().FieldFunc("s2root", func() string {
		return "hello"
	})

	foo := schema.Object("Foo", Foo{})

	foo.FieldFunc("s2ok", func(ctx context.Context, in *Foo) (int, error) {
		return len(in.Name), nil
	})

	foo.FieldFunc("s2bar", func(in *Foo) *Bar {
		return &Bar{
			Id: int64(len(in.Name)*2 + 4),
		}
	})

	foo.FieldFunc("s2nest", func(f *Foo) *Foo {
		return f
	})

	bar := schema.Object("Bar", Bar{})
	bar.Federation(func(b *Bar) int64 {
		return b.Id
	})

	return schema
}

func mustParse(s string) *graphql.RawSelectionSet {
	return graphql.MustParse(s, map[string]interface{}{}).SelectionSet
}

func TestMustParse(t *testing.T) {
	query := mustParse(`
		{
			fff {
				hmm
				ah: ok
				bar {
					id
					baz
				}
			}
		}
	`)

	expected := &graphql.RawSelectionSet{
		Selections: []*graphql.RawSelection{
			{
				Name:  "fff",
				Alias: "fff",
				Args:  map[string]interface{}{},
				SelectionSet: &graphql.RawSelectionSet{
					Selections: []*graphql.RawSelection{
						{
							Name:  "hmm",
							Alias: "hmm",
							Args:  map[string]interface{}{},
						},
						{
							Name:  "ok",
							Alias: "ah",
							Args:  map[string]interface{}{},
						},
						{
							Name:  "bar",
							Alias: "bar",
							Args:  map[string]interface{}{},
							SelectionSet: &graphql.RawSelectionSet{
								Selections: []*graphql.RawSelection{
									{
										Name:  "id",
										Alias: "id",
										Args:  map[string]interface{}{},
									},
									{
										Name:  "baz",
										Alias: "baz",
										Args:  map[string]interface{}{},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	assert.Equal(t, expected, query)
}

func roundtripJson(t *testing.T, v interface{}) interface{} {
	bytes, err := json.Marshal(v)
	require.NoError(t, err)
	var r interface{}
	err = json.Unmarshal(bytes, &r)
	require.NoError(t, err)
	return r
}

func assertExecuteEqual(ctx context.Context, t *testing.T, e *Executor, in, out string) {
	plan, err := e.Plan(graphql.MustParse(in, map[string]interface{}{}))
	require.NoError(t, err)

	res, err := e.Execute(ctx, plan)
	require.NoError(t, err)

	var expected interface{}
	err = json.Unmarshal([]byte(out), &expected)
	require.NoError(t, err)

	assert.Equal(t, expected, roundtripJson(t, res))
}

func TestExecutor(t *testing.T) {
	ctx := context.Background()

	// todo: assert specific invocation traces?

	execs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"schema1": buildTestSchema1(),
		"schema2": buildTestSchema2(),
	})
	require.NoError(t, err)

	e, err := NewExecutor(ctx, execs)
	require.NoError(t, err)

	testCases := []struct {
		Name   string
		Input  string
		Output string
	}{
		{
			Name: "kitchen sink",
			Input: `
				{
					s1fff {
						a: s1nest { b: s1nest { c: s1nest { s2ok } } }
						s1hmm
						s1enum
						s2ok
						s2bar {
							id
							s1baz
						}
						s1nest {
							name
						}
						s2nest {
							name
						}
					}
					s1echo(foo: "foo", required: {a: 1, b: 3})
					s1both {
						... on Foo {
							name
							s1hmm
							s2ok
							a: s1nest { b: s1nest { c: s1nest { s2ok } } }
						}
						... on Bar {
							id
							s1baz
						}
					}
					s2root
				}
			`,
			Output: `{
				"s1fff": [{
					"a": {"b": {"c": {"s2ok": 5}}},
					"s1hmm": "jimbo!!!",
					"s1enum": "one",
					"s2ok": 5,
					"s2bar": {
						"id": 14,
						"s1baz": "14"
					},
					"s1nest": {
						"name": "jimbo"
					},
					"s2nest": {
						"name": "jimbo"
					}
				},
				{
					"a": {"b": {"c": {"s2ok": 3}}},
					"s1hmm": "bob!!!",
					"s1enum": "one",
					"s2ok": 3,
					"s2bar": {
						"id": 10,
						"s1baz": "10"
					},
					"s1nest": {
						"name": "bob"
					},
					"s2nest": {
						"name": "bob"
					}
				}],
				"s1echo": "foo {1 3} <nil>",
				"s1both": [{
					"__typename": "Foo",
					"name": "this is the foo",
					"s1hmm": "this is the foo!!!",
					"a": {"b": {"c": {"s2ok": 15}}},
					"s2ok": 15
				},
				{
					"__typename": "Bar",
					"id": 1234,
					"s1baz": "1234"
				}],
				"s2root": "hello"
			}`,
		},
		{
			Name: "mutation",
			Input: `mutation Test {
				s1addFoo(name: "test") {
					name
					s2ok
				}
			}`,
			Output: `{
				"s1addFoo": {
					"name": "test",
					"s2ok": 4
				}
			}`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			assertExecuteEqual(ctx, t, e, testCase.Input, testCase.Output)
		})
	}
}

// TestConcurrency tests that plans that can run concurrently do run
// concurrently.
//
// It's not a very exhaustive test.
func TestConcurrency(t *testing.T) {
	type Foo struct {
	}

	var running chan chan struct{}

	makeSchema := func(prefix string) *schemabuilder.Schema {
		schema := schemabuilder.NewSchema()
		schema.Query().FieldFunc(prefix+"foo", func() *Foo {
			return &Foo{}
		})

		foo := schema.Object("Foo", Foo{})
		foo.Federation(func(*Foo) string {
			return ""
		})
		foo.FieldFunc(prefix+"instant", func(f *Foo) *Foo {
			return f
		})
		foo.FieldFunc(prefix+"wait", func(f *Foo) *Foo {
			// xxx: store path in Foo by concatenating field name funcs and then
			// use that to record exec order?
			release := <-running
			<-release
			return f
		})
		foo.FieldFunc(prefix+"echo", func(f *Foo) string {
			return "echo"
		})

		schema.Federation().FieldFunc("Foo", func(args struct{ Keys []string }) []*Foo {
			foos := make([]*Foo, 0, len(args.Keys))
			for range args.Keys {
				foos = append(foos, &Foo{})
			}
			return foos
		})

		return schema
	}

	ctx := context.Background()

	// todo: assert specific invocation traces?

	execs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"schema1": makeSchema("s1"),
		"schema2": makeSchema("s2"),
	})
	require.NoError(t, err)

	e, err := NewExecutor(ctx, execs)
	require.NoError(t, err)

	running = make(chan chan struct{}, 0)
	go func() {
		release := make(chan struct{}, 0)
		for i := 0; i < 3; i++ {
			running <- release
		}
		close(release)
	}()

	assertExecuteEqual(ctx, t, e, `
		{
			s1foo {
				s1instant {
					s2wait {
						s2echo
					}
				}
				s2instant {
					s1wait {
						s1echo
					}
				}
			}
			s2foo {
				s1wait {
					s1echo
				}
			}
		}
	`, `
		{
			"s1foo": {
				"s1instant": {"s2wait": {"s2echo": "echo"}},
				"s2instant": {"s1wait": {"s1echo": "echo"}}
			},
			"s2foo": {
				"s1wait": {"s1echo": "echo"}
			}
		}
	`)

	// xxx: test and verify concurrency limit?
}
