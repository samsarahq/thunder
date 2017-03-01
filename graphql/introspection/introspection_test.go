package introspection_test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"github.com/samsarahq/thunder/graphql/introspection"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
)

type User struct {
	Name     string
	MaybeAge *int64
}

func makeSchema() *schemabuilder.Schema {
	schema := schemabuilder.NewSchema()

	query := schema.Query()
	query.FieldFunc("me", func() User {
		return User{Name: "me"}
	})
	query.FieldFunc("noone", func() *User {
		return nil
	})

	user := schema.Object("User", User{})
	user.FieldFunc("friends", func(u *User) []*User {
		return nil
	})
	user.FieldFunc("greet", func(args struct {
		Other   string
		Include *User
	}) string {
		return ""
	})

	mutation := schema.Mutation()
	mutation.FieldFunc("sayHi", func() {})

	return schema
}

func TestComputeSchemaJSON(t *testing.T) {
	schemaBuilderSchema := makeSchema()

	actualBytes, err := introspection.ComputeSchemaJSON(*schemaBuilderSchema)
	if err != nil {
		t.Fatal(err)
	}
	var actual map[string]interface{}
	json.Unmarshal(actualBytes, &actual)

	if os.Getenv("UPDATE_TEST_RESULTS") != "" {
		ioutil.WriteFile("test-schema.json", actualBytes, 0644)
	}

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
