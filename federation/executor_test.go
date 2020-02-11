package federation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeExecutors(schemas map[string]*schemabuilder.Schema) (map[string]ExecutorClient, error) {
	executors := make(map[string]ExecutorClient)

	for name, schema := range schemas {
		srv, err := NewServer(schema.MustBuild())
		if err != nil {
			return nil, err
		}
		executors[name] = &DirectExecutorClient{Client: srv}
	}

	return executors, nil
}

func roundtripJson(t *testing.T, v interface{}) interface{} {
	bytes, err := json.Marshal(v)
	require.NoError(t, err)
	var r interface{}
	err = json.Unmarshal(bytes, &r)
	require.NoError(t, err)
	return r
}



func runAndValidateQueryResults(t *testing.T, ctx context.Context, e *Executor, query string, out string) {
	res, err := e.Execute(ctx, graphql.MustParse(query, map[string]interface{}{}))
	var expected interface{}
	err = json.Unmarshal([]byte(out), &expected)
	require.NoError(t, err)
	assert.Equal(t,expected, res)
}


func TestExecutorWithFederatedObject(t *testing.T) {
	// The first schema has a user object with an id and orgId
	type User struct {
		Id int64
		OrgId int64
	}
	s1 := schemabuilder.NewSchema()
	user := s1.Object("User", User{})
	user.Key("id")
	user.Federation(func(u *User) int64 {
		return u.Id
	})
	s1.Query().FieldFunc("users", func(ctx context.Context) ([]*User, error) {
		users := make([]*User, 0, 1)
		users = append(users, &User{Id:int64(1), OrgId:int64(9086)})
		return users, nil
	})

	// The second schema has a user with an email and a secret field
	type UserWithEmail struct {
		Id int64
		OrgId int64
		Email string
	}
	s2 := schemabuilder.NewSchema()
	s2.Federation().FieldFunc("User", func(args struct{ Keys []int64 }) []*UserWithEmail {
		users := make([]*UserWithEmail, 0, len(args.Keys))
		users = append(users, &UserWithEmail{Id: int64(1), Email: "yaaayeeeet@gmail.com"})
		return users
	})
	user2 := s2.Object("User", UserWithEmail{})
	user2.FieldFunc("secret", func(ctx context.Context, user *UserWithEmail) (string, error) {
		return "shhhhh", nil
	})

	ctx := context.Background()
	execs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"s1": s1,
		"s2": s2,
	})
	require.NoError(t, err)

	e, err := NewExecutor(ctx, execs)
	require.NoError(t, err)

	query := `
	query Foo {
		users {
			id
			orgId
			email
			secret
		}
	}
`

	out := `
	{
		"users":[
			{
				"__key":1,
				"email":"yaaayeeeet@gmail.com",
				"id":1,
				"orgId":9086,
				"secret":"shhhhh"
			}
		]
	}
`
	runAndValidateQueryResults(t, ctx, e, query, out)
}
