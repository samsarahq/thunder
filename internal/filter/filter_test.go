package filter_test

import (
	"testing"

	"github.com/samsarahq/thunder/internal/filter"
	"github.com/stretchr/testify/assert"
)

func TestMatch(t *testing.T) {
	testcases := []struct {
		String  string
		Query   string
		Matches bool
	}{
		{"hi, san francisco", `"san fran"`, true},
		{"hi, San Francisco", `"san fran"`, true},
		{"hi, San Francisco", `"SAN FRAN"`, true},
		{"hi, san francisco", `"san fran" and`, true},
		{"hi, sandy francisco", `"san fran"`, false},
		{"hi, sandy francisco", `"san fran" and`, true},
		{"hi, sandy francisco", ``, true},
		{"hi, sandy francisco", `""`, false},
	}

	for _, tc := range testcases {
		matchStrings := filter.GetMatchStrings(tc.Query)
		assert.Equal(t,
			tc.Matches,
			filter.MatchText(tc.String, matchStrings),
			"expected Match(`%s`, `%s`) to be %v", tc.String, tc.Query, tc.Matches,
		)
	}
}
