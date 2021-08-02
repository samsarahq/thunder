package graphql_test

import (
	"github.com/samsarahq/thunder/graphql"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSelectionSetShallowCopy(t *testing.T) {
	testCases := []*graphql.SelectionSet{
		{},
		{Selections: []*graphql.Selection{{Name: "test"}}, Fragments: []*graphql.Fragment{{On: "test"}}},
		{Selections: []*graphql.Selection{{Name: "test"}}},
		{Fragments: []*graphql.Fragment{{On: "test"}}},
	}

	for _, tc := range testCases {
		r := tc.ShallowCopy()
		if tc.Fragments == nil {
			require.Nil(t, r.Fragments)
		} else {
			require.True(t, &tc.Fragments != &r.Fragments)
			require.True(t, len(tc.Fragments) == len(r.Fragments))
			for i, f := range r.Fragments {
				require.Equal(t, f, tc.Fragments[i])
			}
		}
		if tc.Selections == nil {
			require.Nil(t, r.Selections)
		} else {
			require.True(t, &tc.Selections != &r.Selections)
			require.True(t, len(tc.Selections) == len(r.Selections))
			for i, f := range r.Selections {
				require.Equal(t, f, tc.Selections[i])
			}
		}
	}
}
