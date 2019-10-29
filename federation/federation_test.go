package main

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

	foo := schema.Object("foo", Foo{})
	foo.BatchFieldFunc("s1hmm", func(ctx context.Context, in map[batch.Index]*Foo) (map[batch.Index]string, error) {
		out := make(map[batch.Index]string)
		for i, foo := range in {
			out[i] = foo.Name + "!!!"
		}
		return out, nil
	})
	foo.FieldFunc("federationKey", func(f *Foo) string {
		return f.Name
	})

	foo.FieldFunc("s1nest", func(f *Foo) *Foo {
		return f
	})

	schema.Query().FieldFunc("barsFromFederationKeys", func(args struct{ Keys []int64 }) []*Bar {
		bars := make([]*Bar, 0, len(args.Keys))
		for _, key := range args.Keys {
			bars = append(bars, &Bar{Id: key})
		}
		return bars
	})

	bar := schema.Object("bar", Bar{})
	bar.FieldFunc("s1baz", func(b *Bar) string {
		return fmt.Sprint(b.Id)
	})

	return schema
}

func buildTestSchema2() *schemabuilder.Schema {
	schema := schemabuilder.NewSchema()

	schema.Query().FieldFunc("foosFromFederationKeys", func(args struct{ Keys []string }) []*Foo {
		foos := make([]*Foo, 0, len(args.Keys))
		for _, key := range args.Keys {
			foos = append(foos, &Foo{Name: key})
		}
		return foos
	})

	foo := schema.Object("foo", Foo{})

	// XXX: require schema.Key

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

	bar := schema.Object("bar", Bar{})
	bar.FieldFunc("federationKey", func(b *Bar) int64 {
		return b.Id
	})

	return schema
}

func mustParse(s string) []*Selection {
	return convert(graphql.MustParse(s, map[string]interface{}{}).SelectionSet)
}

/*
func TestBuildSchema(t *testing.T) {
	schemas := map[string]*schemabuilder.Schema{
		"schema1": buildTestSchema1(),
		"schema2": buildTestSchema2(),
	}

	types := convertSchema(mustExtractSchemas(schemas))

	expected := map[TypeName]*Object{
		"Query": {
			Fields: map[string]*Field{
				"s1f": {
					Service: "schema1",
					Services: map[string]bool{
						"schema1": true,
					},
					Args: nil,
					Type: "foo",
				},
				"s1fff": {
					Service: "schema1",
					Services: map[string]bool{
						"schema1": true,
					},
					Args: nil,
					Type: "foo",
				},
				// XXX: federate other directon as well!
				// XXX: federate multiple types?
				"foosFromFederationKeys": {
					Service: "schema2",
					Services: map[string]bool{
						"schema2": true,
					},
					Args: nil, // XXX
					Type: "foo",
				},
				"barsFromFederationKeys": {
					Service: "schema1",
					Services: map[string]bool{
						"schema1": true,
					},
					Args: nil, // XXX
					Type: "bar",
				},
			},
		},
		"Mutation": {
			Fields: map[string]*Field{},
		},
		"foo": {
			Fields: map[string]*Field{
				"name": {
					Service: "schema1",
					Services: map[string]bool{
						"schema1": true,
						"schema2": true,
					},
					Type: "string",
				},
				"federationKey": {
					Service: "schema1",
					Services: map[string]bool{
						"schema1": true,
					},
					Args: nil,
					Type: "string",
				},
				"s1hmm": {
					Service: "schema1",
					Services: map[string]bool{
						"schema1": true,
					},
					Args: nil,
					Type: "string",
				},
				"s1nest": {
					Service: "schema1",
					Services: map[string]bool{
						"schema1": true,
					},
					Args: nil,
					Type: "foo",
				},
				"s2ok": {
					Service: "schema2",
					Services: map[string]bool{
						"schema2": true,
					},
					Args: nil,
					Type: "int",
				},
				"s2bar": {
					Service: "schema2",
					Services: map[string]bool{
						"schema2": true,
					},
					Type: "bar",
				},
				"s2nest": {
					Service: "schema2",
					Services: map[string]bool{
						"schema2": true,
					},
					Args: nil,
					Type: "foo",
				},
			},
		},
		"bar": {
			Fields: map[string]*Field{
				"id": {
					Service: "schema1",
					Services: map[string]bool{
						"schema1": true,
						"schema2": true,
					},
					Type: "int64",
				},
				"federationKey": {
					Service: "schema2",
					Services: map[string]bool{
						"schema2": true,
					},
					Args: nil,
					Type: "int64",
				},
				"s1baz": {
					Service: "schema1",
					Services: map[string]bool{
						"schema1": true,
					},
					Args: nil,
					Type: "string",
				},
			},
		},
	}

	assert.Equal(t, expected, types)
}
*/

func TestPlan(t *testing.T) {
	schemas := map[string]*schemabuilder.Schema{
		"schema1": buildTestSchema1(),
		"schema2": buildTestSchema2(),
	}

	types := convertSchema(mustExtractSchemas(schemas))

	e := &Executor{
		schema: types,
	}

	testCases := []struct {
		Name   string
		Input  string
		Output []*Plan
	}{
		{
			Name: "kitchen sink",
			Input: `
				{
					s1fff {
						a: s1nest { b: s1nest { c: s1nest { s2ok } } }
						s1hmm
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
				}
			`,
			Output: []*Plan{
				{
					Path:    nil,
					Service: "schema1",
					Type:    "Query",
					Selections: mustParse(`{
						s1fff {
							a: s1nest { b: s1nest { c: s1nest { federationKey } } }
							s1hmm
							s1nest {
								name
							}
							federationKey
						}
					}`),
					After: []*Plan{
						{
							Path:    []string{"s1fff", "a", "b", "c"},
							Type:    "foo",
							Service: "schema2",
							Selections: mustParse(`{
								s2ok
							}`),
						},
						{
							Path:    []string{"s1fff"},
							Type:    "foo",
							Service: "schema2",
							Selections: mustParse(`{
								s2ok
								s2bar {
									id
									federationKey
								}
								s2nest {
									name
								}
							}`),
							After: []*Plan{
								{
									Path:    []string{"s2bar"},
									Type:    "bar",
									Service: "schema1",
									Selections: mustParse(`{
										s1baz
									}`),
								},
							},
						},
					},
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			plan, err := e.Plan(e.schema.Query, mustParse(testCase.Input))
			require.NoError(t, err)
			assert.Equal(t, testCase.Output, plan.After)
		})
	}
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

	expected := []*Selection{
		{
			Name:  "fff",
			Alias: "fff",
			Args:  map[string]interface{}{},
			Selections: []*Selection{
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
					Selections: []*Selection{
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

func TestExecutor(t *testing.T) {
	ctx := context.Background()

	// todo: assert specific invocation traces?

	execs, close, err := makeExecutors(map[string]*schemabuilder.Schema{
		"schema1": buildTestSchema1(),
		"schema2": buildTestSchema2(),
	})
	require.NoError(t, err)
	defer close()

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
				}
			`,
			Output: `{
				"s1fff": [{
					"a": {"b": {"c": {"federationKey": "jimbo", "s2ok": 5}}},
					"s1hmm": "jimbo!!!",
					"s2ok": 5,
					"s2bar": {
						"id": 14,
						"federationKey": 14,
						"s1baz": "14"
					},
					"s1nest": {
						"name": "jimbo"
					},
					"s2nest": {
						"name": "jimbo"
					},
					"federationKey": "jimbo"
				},
				{
					"a": {"b": {"c": {"federationKey": "bob", "s2ok": 3}}},
					"s1hmm": "bob!!!",
					"s2ok": 3,
					"s2bar": {
						"id": 10,
						"federationKey": 10,
						"s1baz": "10"
					},
					"s1nest": {
						"name": "bob"
					},
					"s2nest": {
						"name": "bob"
					},
					"federationKey": "bob"
				}]
			}`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			plan, err := e.Plan(e.schema.Query, mustParse(testCase.Input))
			require.NoError(t, err)

			res, err := e.execute(ctx, plan.After[0], nil)
			require.NoError(t, err)

			var expected interface{}
			err = json.Unmarshal([]byte(testCase.Output), &expected)
			require.NoError(t, err)

			assert.Equal(t, expected, roundtripJson(t, res[0]))
		})
	}
}
