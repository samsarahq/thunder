package federation

import (
	"testing"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustParse(s string) *graphql.SelectionSet {
	return graphql.MustParse(s, map[string]interface{}{}).SelectionSet
}

func setupExecutor(t *testing.T) (*Planner, error) {
	schemas := map[string]map[string]*schemabuilder.Schema{
		"schema1": {
			"schema1": buildTestSchema1(),
		},
		"schema2": {
			"schema2": buildTestSchema2(),
		},
	}

	builtSchemas := make(serviceSchemas)
	for service, versions := range schemas {
		builtSchemas[service] = make(map[string]*introspectionQueryResult)
		for version, schema := range versions {
			builtSchemas[service][version] = extractSchema(t, schema.MustBuild())
		}
	}
	merged, err := convertVersionedSchemas(builtSchemas)
	require.NoError(t, err)

	f, err := newFlattener(merged.Schema)
	return &Planner{
		flattener: f,
		schema:    merged,
	}, nil
}

func TestPlanner(t *testing.T) {
	e, err := setupExecutor(t)
	require.NoError(t, err)

	testCases := []struct {
		Name   string
		Input  string
		Output []*Plan
	}{
		{
			Name: "query with fields from the same service",
			Input: `query Test {
				s1fff {
					name
				}
			}`,
			Output: []*Plan{
				{
					Service: "schema1",
					Type:    "Query",
					Kind:    "query",
					SelectionSet: mustParse(`{
						s1fff {
							name
						}
					}`),
					After: nil,
				},
			},
		},
		{
			Name: "query with fields from two seperate services",
			Input: `query Test {
				s1fff {
					name
					s2ok
				}
			}`,
			Output: []*Plan{
				{
					Service: "schema1",
					Type:    "Query",
					Kind:    "query",
					SelectionSet: mustParse(`{
						s1fff {
							name
							__federation
						}
					}`),
					After: []*Plan{
						{
							Path: []PathStep{
								{Kind: KindField, Name: "s1fff"},
							},
							Type:    "Foo",
							Kind:    "query",
							Service: "schema2",
							SelectionSet: mustParse(`{
								s2ok
							}`),
						},
					},
				},
			},
		},
		{
			Name: "query with fields from nested levels between services",
			Input: `query Test {
				s1fff {
					s2bar {
						s1baz
					}
				}
			}`,
			Output: []*Plan{
				{
					Service: "schema1",
					Type:    "Query",
					Kind:    "query",
					SelectionSet: mustParse(`{
						s1fff {
							__federation
						}
					}`),
					After: []*Plan{
						{
							Path: []PathStep{
								{Kind: KindField, Name: "s1fff"},
							},
							Type:    "Foo",
							Kind:    "query",
							Service: "schema2",
							SelectionSet: mustParse(`{
								s2bar {
									__federation
								}
							}`),
							After: []*Plan{
								{
									Path: []PathStep{
										{Kind: KindField, Name: "s2bar"},
									},
									Type:    "Bar",
									Kind:    "query",
									Service: "schema1",
									SelectionSet: mustParse(`{
										s1baz
									}`),
								},
							},
						},
					},
				},
			},
		},
		{
			Name: "with union types resolved by different services",
			Input: `query Test {
				s1both {
					__typename
					... on Foo {
						a: s1nest { b: s1nest { c: s1nest { s2ok } } }
						name
						s1hmm
						s2ok
					}
					... on Bar {
						id
						s1baz
					}
				}
			}`,
			Output: []*Plan{
				{
					Service: "schema1",
					Type:    "Query",
					Kind:    "query",
					SelectionSet: mustParse(`{
						s1both {
							__typename
							... on Bar {
								__typename
								id
								s1baz
							}
							... on Foo {
								__typename
								a: s1nest { b: s1nest { c: s1nest { __federation } } }
								name
								s1hmm
								__federation
							}
						}
					}`),
					After: []*Plan{
						
						{
							Path: []PathStep{
								{Kind: KindField, Name: "s1both"},
								{Kind: KindType, Name: "Foo"},
								{Kind: KindField, Name: "a"},
								{Kind: KindField, Name: "b"},
								{Kind: KindField, Name: "c"},
							},
							Type:    "Foo",
							Kind:    "query",
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
							Kind:    "query",
							Service: "schema2",
							SelectionSet: mustParse(`{
								s2ok
							}`),
						},
					},
				},
			},
		},
		{
			Name: "kitchen sink query",
			Input: `query Test {
				s1echo(foo: "foo", pair: {a: 1, b: 3})
				s1fff {
					a: s1nest { b: s1nest { c: s1nest { s2ok } } }
					s1hmm
					s1nest {
						name
					}
					s2bar {
						id
						s1baz
					}
					s2nest {
						name
					}
					s2ok
				}
				s2root
			}`,
			Output: []*Plan{
				{
					Service: "schema1",
					Type:    "Query",
					Kind:    "query",
					SelectionSet: mustParse(`{
						s1echo(foo: "foo", pair: {a: 1, b: 3})
						s1fff {
							a: s1nest { b: s1nest { c: s1nest { __federation } } }
							s1hmm
							s1nest {
								name
							}
							__federation
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
							Kind:    "query",
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
							Kind:    "query",
							Service: "schema2",
							SelectionSet: mustParse(`{
								s2bar {
									id
									__federation
								}
								s2nest {
									name
								}
								s2ok
							}`),
							After: []*Plan{
								{
									Path: []PathStep{
										{Kind: KindField, Name: "s2bar"},
									},
									Type:    "Bar",
									Kind:    "query",
									Service: "schema1",
									SelectionSet: mustParse(`{
										s1baz
									}`),
								},
							},
						},
					},
				},
				{
					Service: "schema2",
					Type:    "Query",
					Kind:    "query",
					SelectionSet: mustParse(`{
						s2root
					}`),
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
