package graphql_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kylelemons/godebug/pretty"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
)

func testHTTPRequest(req *http.Request) *httptest.ResponseRecorder {
	schema := schemabuilder.NewSchema()

	query := schema.Query()
	query.FieldFunc("mirror", func(args struct{ Value int64 }) int64 {
		return args.Value * -1
	})

	builtSchema := schema.MustBuild()

	rr := httptest.NewRecorder()
	handler := graphql.HTTPHandler(builtSchema)

	handler.ServeHTTP(rr, req)
	return rr
}

func TestHTTPMustPost(t *testing.T) {
	req, err := http.NewRequest("GET", "/graphql", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := testHTTPRequest(req)

	if rr.Code != 200 {
		t.Errorf("expected 200, but received %d", rr.Code)
	}

	if diff := pretty.Compare(rr.Body.String(), "{\"data\":null,\"errors\":[\"request must be a POST\"]}"); diff != "" {
		t.Errorf("expected response to match, but received %s", diff)
	}
}

func TestHTTPParseQuery(t *testing.T) {
	req, err := http.NewRequest("POST", "/graphql", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := testHTTPRequest(req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, but received %d", rr.Code)
	}

	if diff := pretty.Compare(rr.Body.String(), "{\"data\":null,\"errors\":[\"request must include a query\"]}"); diff != "" {
		t.Errorf("expected response to match, but received %s", diff)
	}
}

func TestHTTPMustHaveQuery(t *testing.T) {
	req, err := http.NewRequest("POST", "/graphql", strings.NewReader(`{"query":""}`))
	if err != nil {
		t.Fatal(err)
	}

	rr := testHTTPRequest(req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, but received %d", rr.Code)
	}

	if diff := pretty.Compare(rr.Body.String(), "{\"data\":null,\"errors\":[\"must have a single query\"]}"); diff != "" {
		t.Errorf("expected response to match, but received %s", diff)
	}
}

func TestHTTPSuccess(t *testing.T) {
	req, err := http.NewRequest("POST", "/graphql", strings.NewReader(`{"query": "query TestQuery($value: int64) { mirror(value: $value) }", "variables": { "value": 1 }}`))
	if err != nil {
		t.Fatal(err)
	}

	rr := testHTTPRequest(req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, but received %d", rr.Code)
	}

	if diff := pretty.Compare(rr.Body.String(), "{\"data\":{\"mirror\":-1},\"errors\":null}"); diff != "" {
		t.Errorf("expected response to match, but received %s", diff)
	}
}

func TestHTTPContentType(t *testing.T) {
	req, err := http.NewRequest("POST", "/graphql", strings.NewReader(`{"query": "query TestQuery($value: int64) { mirror(value: $value) }", "variables": { "value": 1 }}`))
	if err != nil {
		t.Fatal(err)
	}

	rr := testHTTPRequest(req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, but received %d", rr.Code)
	}

	if diff := pretty.Compare(rr.HeaderMap.Get("Content-Type"), "application/json"); diff != "" {
		t.Errorf("expected response to match, but received %s", diff)
	}
}

func TestHTTPWrongJSONStructure(t *testing.T) {
	testcases := []struct {
		Title        string
		Request      string
		ExpectedBody string
	}{
		{
			Title:        "Missing 'query' field",
			Request:      `{"variables": { "value": 1 }}`,
			ExpectedBody: `{"data":null,"errors":["must have a single query"]}`,
		},
		{
			Title:        "Incorrect JSON structure: double 'query'",
			Request:      `{"query": { "foo": 1 }, "query": "query TestQuery($value: int64) { mirror(value: $value) }", "variables": { "value": 3 }}`,
			ExpectedBody: `{"data":null,"errors":["request must has a body with valid JSON structure"]}`,
		},
		{
			Title:        "Missing 'query' field",
			Request:      `{"query": { "foo": 1 }, "query": "query TestQuery($value: int64) { mirror(value: $value) }"}`,
			ExpectedBody: `{"data":null,"errors":["request must has a body with valid JSON structure"]}`,
		},
		{
			Title:        "Missing 'variables' field",
			Request:      `{"query": "query TestQuery($value: int64) { mirror(value: $value) }", "var": { "value": 1 }}`,
			ExpectedBody: `{"data":null,"errors":["error parsing args for \"mirror\": value: not a number"]}`,
		},
		{
			Title:        "Missing 'query' and 'variables' fields",
			Request:      `{"foo": { "bar": 1 }}`,
			ExpectedBody: `{"data":null,"errors":["must have a single query"]}`,
		},
		{
			Title:        "Missing everything",
			Request:      `{}`,
			ExpectedBody: `{"data":null,"errors":["must have a single query"]}`,
		},
		{
			Title:        "Missing everything, even curly brackets",
			Request:      ``,
			ExpectedBody: `{"data":null,"errors":["request must has a body with valid JSON structure"]}`,
		},
		{
			Title:        "Field 'query' has wrong structure",
			Request:      `{"query": { "query": "query TestQuery($value: int64) { mirror(value: $value) }" }, "variables": { "value": 1 }}`,
			ExpectedBody: `{"data":null,"errors":["request must has a body with valid JSON structure"]}`,
		},
		{
			Title:        "Field 'variables' is not an object",
			Request:      `{"query": "query TestQuery($value: int64) { mirror(value: $value) }", "variables": "{ "value": 1 }"}`,
			ExpectedBody: `{"data":null,"errors":["request must has a body with valid JSON structure"]}`,
		},
		{
			Title:        "Everything is fine",
			Request:      `{"query": "query TestQuery($value: int64) { mirror(value: $value) }", "variables": { "value": 1 }}`,
			ExpectedBody: `{"data":{"mirror":-1},"errors":null}`,
		},
	}

	for _, testcase := range testcases {
		req, err := http.NewRequest("POST", "/graphql", strings.NewReader(testcase.Request))
		if err != nil {
			t.Errorf("%s clause: cannot start request, got an error '%s'", testcase.Title, err)
		}
		rr := testHTTPRequest(req)
		body := rr.Body.String()
		if diff := pretty.Compare(body, testcase.ExpectedBody); diff != "" {
			t.Errorf("%s clause: got '%s', when it expected as '%s'", testcase.Title, body, testcase.ExpectedBody)
		}
	}
}
