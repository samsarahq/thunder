package federation

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/samsarahq/thunder/internal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPRequest(t *testing.T) {
	ctx := context.Background()

	execs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"schema1": buildTestSchema1(),
		"schema2": buildTestSchema2(),
	})
	require.NoError(t, err)

	e, err := NewExecutor(ctx, execs)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := HTTPHandler(e)

	req, err := http.NewRequest("POST", "/graphql", strings.NewReader(`{"query":"{ s1fff { name s1hmm s2ok } }"}`))
	assert.NoError(t, err)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.HeaderMap.Get("Content-Type"))
	assert.Equal(t, internal.ParseJSON(`
	{
		"data": {
			"s1fff": [{
				"name": "jimbo",
				"s1hmm": "jimbo!!!",
				"s2ok": 5
			},
			{
				"name": "bob",
				"s1hmm": "bob!!!",
				"s2ok": 3
			}]
		},
		"errors": null
	}`), internal.ParseJSON(rr.Body.String()))
}
