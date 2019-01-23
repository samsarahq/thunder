package introspection_test

import (
	"encoding/json"
	"testing"

	"github.com/samsarahq/go/snapshotter"
	"github.com/samsarahq/thunder/graphql/introspection"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/stretchr/testify/require"
)

type User struct {
	Name     string
	MaybeAge *int64
	Uuid     Uuid
}

type Vehicle struct {
	Name  string
	Speed int64
	Uuid  Uuid
}
type Asset struct {
	Name         string
	BatteryLevel int64
	Uuid         Uuid
}

type Gateway struct {
	schemabuilder.Union

	*Vehicle
	*Asset
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
	query.FieldFunc("usersConnection", func() ([]User, error) {
		return nil, nil
	}, schemabuilder.Paginated)
	query.FieldFunc("usersConnectionPtr", func() ([]*User, error) {
		return nil, nil
	}, schemabuilder.Paginated)
	query.FieldFunc("userUuid", func() (*Uuid, error) {
		return nil, nil
	})
	query.FieldFunc("usersUuid", func() ([]Uuid, error) {
		return nil, nil
	})

	query.FieldFunc("gateway", func() (*Gateway, error) {
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
		Optional  string `graphql:",optional"`
	}) string {
		return ""
	})

	mutation := schema.Mutation()
	mutation.FieldFunc("sayHi", func() {})

	return schema
}

func TestComputeSchemaJSON(t *testing.T) {
	snap := snapshotter.New(t)
	defer snap.Verify()
	schemaBuilderSchema := makeSchema()

	actualBytes, err := introspection.ComputeSchemaJSON(*schemaBuilderSchema)
	require.NoError(t, err)

	var actual map[string]interface{}
	json.Unmarshal(actualBytes, &actual)
	snap.Snapshot("schema", actual)
}

// Uuid is a stub version of a "Text Marshalable" type.
type Uuid struct{}

func (u Uuid) MarshalText() ([]byte, error) {
	return nil, nil
}

func (u *Uuid) UnmarshalText(data []byte) error {
	return nil
}
