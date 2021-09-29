package filter_test

import (
	"testing"

	"github.com/samsarahq/thunder/internal/filter"
	"github.com/stretchr/testify/assert"
)

func TestDefaultMatchText(t *testing.T) {
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
		searchTokens := filter.GetDefaultSearchTokens(tc.Query)
		assert.Equal(t,
			tc.Matches,
			filter.DefaultFilterFunc(tc.String, searchTokens),
			"expected Match(`%s`, `%s`) to be %v", tc.String, tc.Query, tc.Matches,
		)
	}
}
