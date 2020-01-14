package federation

import (
	"testing"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalize(t *testing.T) {
	parse := func(q string) *graphql.SelectionSet {
		return graphql.MustParse(q, map[string]interface{}{}).SelectionSet
	}

	type User struct {
		Name string
	}
	type House struct {
		Name string
	}
	type UserOrHouse struct {
		schemabuilder.Union
		*User
		*House
	}
	sb := schemabuilder.NewSchema()
	sb.Query().FieldFunc("users", func() []*User { return nil })
	sb.Query().FieldFunc("search", func() []*UserOrHouse { return nil })
	sb.Object("User", User{}).FieldFunc("friends", func(args struct{ Limit int64 }) []*User { return nil })
	sb.Object("User", User{}).FieldFunc("self", func(u *User) *User { return u }, schemabuilder.NonNullable)
	sb.Object("House", House{}).FieldFunc("users", func() []*User { return nil })
	schema, err := sb.Build()
	require.NoError(t, err)
	f, err := newFlattener(schema)
	require.NoError(t, err)

	testCases := []struct {
		name   string
		input  string
		output string
		error  string
	}{
		{
			name: "trivial",
			input: `
				{
					users {
						name
					}
				}
			`,
			output: `
				{
					users {
						name
					}
				}
			`,
		},
		{
			name: "just a fragment",
			input: `
				{
					users {
						... on User {
							name
						}
					}
				}
			`,
			output: `
				{
					users {
						name
					}
				}
			`,
		},
		{
			name: "dedup",
			input: `
				{
					users {
						name
						name
						allFriends: friends { name }
						foo: name
						name
						name
						friends(limit: 10) { name }
						name
						foo: name
						friends(limit: 10) { name }
						allFriends: friends { name }
					}
				}
			`,
			output: `
				{
					users {
						allFriends: friends { name }
						foo: name
						friends(limit: 10) { name }
						name
					}
				}
			`,
		},
		{
			name: "dedup nested",
			input: `
				{
					users {
						friends(limit: 10) { foo: name name }
						friends(limit: 10) { bar: name name }
					}
				}
			`,
			output: `
				{
					users {
						friends(limit: 10) { bar: name foo: name name }
					}
				}
			`,
		},
		{
			name: "dedup fragments",
			input: `
				{
					users {
						name
						... on User {
							name
							... on User {
								name
							}
							... Foo
						}
					}
				}

				fragment Foo on User {
					name
				}
			`,
			output: `
				{
					users {
						name
					}
				}
			`,
		},
		{
			name: "mismatched names",
			input: `
				{
					users {
						foo: name
						foo: self { name }
					}
				}
			`,
			error: "two selections with same alias (foo) have different names (self and name)",
		},
		{
			name: "mismatched arguments",
			input: `
				{
					users {
						friends { name }
						friends(limit: 10) { name }
					}
				}
			`,
			error: "two selections with same alias (friends) have different arguments (map[limit:10] and map[])",
		},
		{
			name: "mismatched subselections",
			input: `
				{
					users {
						foo: self
						foo: self { name }
					}
				}
			`,
			error: "one selection with alias foo has subselections and one does not",
		},
		{
			name: "union",
			input: `
				{
					search {
						__typename
					}
				}
			`,
			output: `
				{
					search {
						... on House {
							__typename
						}
						... on User {
							__typename
						}
					}
				}
			`,
		},
		{
			name: "union dedup and inline",
			input: `
				{
					search {
						__typename
					}
					search {
						... on House {
							name
						}
					}
					search {
						... on House {
							users {
								name
								name
							}
						}
					}
					search {
						... on User {
							name
						}
					}
				}
			`,
			output: `
				{
					search {
						... on House {
							__typename
							name
							users {
								name
							}
						}
						... on User {
							__typename
							name
						}
					}
				}
			`,
		},
		{
			name: "kitchen sink",
			input: `
				query Foo {
					users {
						__typename
						... on User {
							name
							... on User {
								... Bar
							}
						}
						name
						someFriends: friends(limit: 10) {
							name
							... Bar
						}
						name
						friends {
							name
						}
						friends {
							... Bar
						}
					}
				}

				fragment Bar on User {
					name
				}
			`,
			output: `
				{
					users {
						__typename
						friends {
							name
						}
						name
						someFriends: friends(limit: 10) {
							name
						}
					}
				}
			`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			flat, err := f.flatten(parse(testCase.input), schema.Query)
			if testCase.error != "" {
				require.EqualError(t, err, testCase.error)
			} else {
				require.NoError(t, err)
				assert.Equal(t, parse(testCase.output), flat)
			}
		})
	}
}
