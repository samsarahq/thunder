package federation

import (
	"context"
	"fmt"
	// "encoding/json"
	// "errors"
	"testing"

	// "github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	// "github.com/samsarahq/thunder/reactive"
	// "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)



func createExecutorWithFederatedObjects2() (*Executor, error) {
	// The first schema has a user object with an id and orgId
	type User struct {
		Id    int64
		OrgId int64
	}
	s1 := schemabuilder.NewSchema()
	user := s1.Object("User", User{})
	user.Key("id")
	user.AddFederatedKey("yeet")

	type UserIds struct {
		Id    int64
		OrgId int64
	}

	user.Federation(func(u *User) *UserIds {
		return &UserIds{Id: u.Id, OrgId: u.OrgId}
	})
	s1.Query().FieldFunc("users", func(ctx context.Context) ([]*User, error) {
		users := make([]*User, 0, 1)
		users = append(users, &User{Id: int64(1), OrgId: int64(9086)})
		return users, nil
	})


	// The second schema has a user with an email and a secret field
	type UserWithEmail struct {
		Id    int64
		OrgId int64
		Email string
	}
	s2 := schemabuilder.NewSchemaWithName("s2")
	// s2.Object("UserIds", UserIds{})
	s2.Federation().FederatedFieldFunc("User", func(args struct{ Keys []UserIds }) []*UserWithEmail {
		fmt.Println("keys", args.Keys)
		users := make([]*UserWithEmail, 0, len(args.Keys))
		users = append(users, &UserWithEmail{Id: int64(1), Email: "yaaayeeeet@gmail.com"})
		return users
	})
	user2 := s2.Object("User", UserWithEmail{})
	user2.FieldFunc("secret", func(ctx context.Context, user *UserWithEmail) (string, error) {
		return "shhhhh", nil
	})


	// The third schema has a user with an email and a secret field
	
	s3 := schemabuilder.NewSchemaWithName("s3")
	// s2.Object("UserIds", UserIds{})
	type UserIds2 struct {
		Id    int64
		OrgId int64
	}
	s3.Federation().FederatedFieldFunc("User", func(args struct{ Keys []UserIds2 }) []*UserWithEmail {
		fmt.Println("keys", args.Keys)
		users := make([]*UserWithEmail, 0, len(args.Keys))
		users = append(users, &UserWithEmail{Id: int64(1), Email: "yaaayeeeet@gmail.com"})
		return users
	})
	// s3.Federation().FieldFunc("UserWithId", func(args struct{ Keys []UserIds2 }) []*UserWithEmail {
	// 	fmt.Println("keys", args.Keys)
	// 	users := make([]*UserWithEmail, 0, len(args.Keys))
	// 	users = append(users, &UserWithEmail{Id: int64(1), Email: "yaaayeeeet@gmail.com"})
	// 	return users
	// })
	user3 := s3.Object("User", UserWithEmail{})
	user3.FieldFunc("supersecret", func(ctx context.Context, user *UserWithEmail) (string, error) {
		return "yeet", nil
	})


	ctx := context.Background()
	execs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"s1": s1,
		"s2": s2,
		"s3": s3,
	})
	if err != nil {
		return nil, err
	}

	return NewExecutor(ctx, execs)
}

func TestExecutorWithFederatedObject2(t *testing.T) {
	e, err := createExecutorWithFederatedObjects2()
	require.NoError(t, err)
	testCases := []struct {
		Name   string
		Query  string
		Output string
	}{
		
		{
			Name: "query with users, fields on both services",
			Query: `
				query Foo {
					users {
						id
						orgId
						email
						secret
						supersecret
					}
				}`,
			Output: `
				{
					"users":[
						{
							"__key":1,
							"email":"yaaayeeeet@gmail.com",
							"id":1,
							"orgId":9086,
							"secret":"shhhhh",
							"supersecret":"yeet"
						}
					]
				}`,
		},
	
	}
	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			// Validates that we were able to execute the query on multiple
			// schemas and correctly stitch the results back together
			ctx := context.Background()
			runAndValidateQueryResults(t, ctx, e, testCase.Query, testCase.Output)
		})
	}
}


