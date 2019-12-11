package federation

import (
	"context"
	"testing"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/samsarahq/thunder/thunderpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlan(t *testing.T) {
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
					s1echo(foo: "foo", pair: {a: 1, b: 3})
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
			Output: []*Plan{
				{
					Service: "schema1",
					Type:    "Query",
					SelectionSet: mustParse(`{
						s1fff {
							a: s1nest { b: s1nest { c: s1nest { __federation } } }
							s1hmm
							s1nest {
								name
							}
							__federation
						}
						s1echo(foo: "foo", pair: {a: 1, b: 3})
						s1both {
							__typename
							... on Bar {
								id
								s1baz
							}
							... on Foo {
								name
								s1hmm
								a: s1nest { b: s1nest { c: s1nest { __federation } } }
								__federation
							}
						}
					}`),
					After: []*Plan{
						{
							Path: []PathStep{
								{Kind: KindField, Name: "s1fff"},
								{Kind: KindField, Name: "a"},
								{Kind: KindField, Name: "b"},
								{Kind: KindField, Name: "c"},
							},
							Type:    "Foo",
							Service: "schema2",
							SelectionSet: mustParse(`{
								s2ok
							}`),
						},
						{
							Path: []PathStep{
								{Kind: KindField, Name: "s1fff"},
							},
							Type:    "Foo",
							Service: "schema2",
							SelectionSet: mustParse(`{
								s2ok
								s2bar {
									id
									__federation
								}
								s2nest {
									name
								}
							}`),
							After: []*Plan{
								{
									Path: []PathStep{
										{Kind: KindField, Name: "s2bar"},
									},
									Type:    "Bar",
									Service: "schema1",
									SelectionSet: mustParse(`{
										s1baz
									}`),
								},
							},
						},
						{
							Path: []PathStep{
								{Kind: KindField, Name: "s1both"},
								{Kind: KindType, Name: "Foo"},
								{Kind: KindField, Name: "a"},
								{Kind: KindField, Name: "b"},
								{Kind: KindField, Name: "c"},
							},
							Type:    "Foo",
							Service: "schema2",
							SelectionSet: mustParse(`{
								s2ok
							}`),
						},
						{
							Path: []PathStep{
								{Kind: KindField, Name: "s1both"},
								{Kind: KindType, Name: "Foo"},
							},
							Type:    "Foo",
							Service: "schema2",
							SelectionSet: mustParse(`{
								s2ok
							}`),
						},
					},
				},
				{
					Service: "schema2",
					Type:    "Query",
					SelectionSet: mustParse(`{
						s2root
					}`),
				},
			},
		},
		{
			Name: "mutation",
			Input: `mutation Test {
				s1addFoo(name: "test") {
					name
					s2ok
				}
			}`,
			Output: []*Plan{
				{
					Service: "schema1",
					Type:    "Mutation",
					Kind:    thunderpb.ExecuteRequest_MUTATION,
					SelectionSet: mustParse(`{
						s1addFoo(name: "test") {
							name
							__federation
						}
					}`),
					After: []*Plan{
						{
							Path: []PathStep{
								{Kind: KindField, Name: "s1addFoo"},
							},
							Type:    "Foo",
							Service: "schema2",
							SelectionSet: mustParse(`{
								s2ok
							}`),
						},
					},
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			plan, err := e.planRoot(graphql.MustParse(testCase.Input, map[string]interface{}{}))
			require.NoError(t, err)
			assert.Equal(t, testCase.Output, plan.After)
		})
	}
}
