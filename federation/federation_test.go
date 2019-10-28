package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/samsarahq/thunder/batch"
	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/stretchr/testify/assert"
)

func buildTestSchema1() *graphql.Schema {
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

	return schema.MustBuild()
}

func buildTestSchema2() *graphql.Schema {
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

	return schema.MustBuild()
}

func mustParse(s string) []*Selection {
	return convert(graphql.MustParse(s, map[string]interface{}{}).SelectionSet)
}

func TestPlan(t *testing.T) {
	executors := map[string]*graphql.Schema{
		"schema1": buildTestSchema1(),
		"schema2": buildTestSchema2(),
	}

	types := convertSchema(executors)

	e := &Executor{
		Types:     types,
		Executors: executors,
	}

	testCases := []struct {
		Name   string
		Input  string
		Output []SubPlan
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
			Output: []SubPlan{
				{
					Path: []string{},
					Plan: &Plan{
						Service: "schema1",
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
						After: []SubPlan{
							{
								Path: []string{"s1fff", "a", "b", "c"},
								Plan: &Plan{
									Type:    "foo",
									Service: "schema2",
									Selections: mustParse(`{
										s2ok
									}`),
								},
							},
							{
								Path: []string{"s1fff"},
								Plan: &Plan{
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
									After: []SubPlan{
										{
											Path: []string{"s2bar"},
											Plan: &Plan{
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
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			plan := e.Plan(e.Types["Query"], mustParse(testCase.Input)).After
			assert.Equal(t, testCase.Output, plan)
		})
	}

}
