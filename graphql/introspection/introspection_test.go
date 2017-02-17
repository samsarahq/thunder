package introspection_test

import (
	"encoding/json"
	"io/ioutil"
	"reflect"
	"testing"

	"github.com/samsarahq/thunder/example"
	"github.com/samsarahq/thunder/graphql/introspection"
)

func TestComputeSchemaJSON(t *testing.T) {
	server := example.Server{}
	schemaBuilderSchema := server.SchemaBuilderSchema()

	actualBytes, err := introspection.ComputeSchemaJSON(*schemaBuilderSchema)
	if err != nil {
		t.Fatal(err)
	}
	var actual map[string]interface{}
	json.Unmarshal(actualBytes, &actual)

	expectedBytes, err := ioutil.ReadFile("test-schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var expected map[string]interface{}
	json.Unmarshal(expectedBytes, &expected)

	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("schema JSONs do not match:\n---expected---\n%+v\n---actual---\n%+v", expected, actual)
	}
}
