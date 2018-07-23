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

type enumType int32

func makeSchema() *schemabuilder.Schema {
	schema := schemabuilder.NewSchema()
	user := schema.Object("user", User{})
	user.Key("name")
	var enumField enumType
	schema.Enum(enumField, map[string]enumType{
		"random":  enumType(3),
		"random1": enumType(2),
		"random2": enumType(1),
	})
	query := schema.Query()
	query.FieldFunc("me", func() User {
		return User{Name: "me"}
	})
	query.FieldFunc("noone", func() *User {
		return &User{Name: "me"}
	}, schemabuilder.NonNullable)
	query.FieldFunc("nullableUser", func() (*User, error) {
		return nil, nil
	})
	query.PaginateFieldFunc("usersConnection", func() ([]User, error) {
		return nil, nil
	})
	query.PaginateFieldFunc("usersConnectionPtr", func() ([]*User, error) {
		return nil, nil
	})

	// Add a non-null field after "noone" to test that caching
	// mechanism in schemabuilder chooses the correct type
	// for the return value.
	query.FieldFunc("viewer", func() (User, error) {
		return User{Name: "me"}, nil
	})

	user.FieldFunc("friends", func(u *User) []*User {
		return nil
	})
	user.FieldFunc("greet", func(args struct {
		Other     string
		Include   *User
		Enumfield enumType
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
