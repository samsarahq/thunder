package federation

import (
	"context"
	"testing"

	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/stretchr/testify/require"
)

func createExecutorWithFederatedObjects2() (*Executor, *schemabuilder.Schema, *schemabuilder.Schema, error) {
	// The first schema has a user object with an id and orgId
	type User struct {
		Id    int64
		OrgId int64
		Name  string
	}
	s1 := schemabuilder.NewSchemaWithName("s1")
	user := s1.Object("User", User{})
	user.Key("id")
	type UserIds struct {
		Id    int64
		OrgId int64
	}
	user.Federation(func(u *User) *User {
		return u
	})
	s1.Query().FieldFunc("users", func(ctx context.Context) ([]*User, error) {
		users := make([]*User, 0, 1)
		users = append(users, &User{Id: int64(1), OrgId: int64(9086)})
		return users, nil
	})

	s1.Query().FieldFunc("usersWithArgs", func(args struct {
		Name string
	}) ([]*User, error) {
		users := make([]*User, 0, 1)
		users = append(users, &User{Id: int64(1), OrgId: int64(9086), Name: args.Name})
		return users, nil
	})

	// The second schema has a user with an email and a secret field
	type UserWithContactInfo struct {
		Id          int64
		OrgId       int64
		Name        string
		Email       string
		PhoneNumber string
	}

	type UserKeys struct {
		Id    int64
		OrgId int64
	}
	s2 := schemabuilder.NewSchemaWithName("s2")
	s2.Federation().FederatedFieldFunc("User", func(args struct{ Keys []UserKeys }) []*UserWithContactInfo {
		users := make([]*UserWithContactInfo, 0, len(args.Keys))
		users = append(users, &UserWithContactInfo{Id: int64(1), Email: "yaaayeeeet@gmail.com", PhoneNumber: "555"})
		return users
	})

	user2 := s2.Object("User", UserWithContactInfo{})
	user2.Key("id")
	user2.FieldFunc("secret", func(ctx context.Context, user *UserWithContactInfo) (string, error) {
		return "shhhhh", nil
	})

	// The second schema has a user with an email and a secret field
	type UserWithAdminPrivelages struct {
		Id      int64
		OrgId   int64
		Name    string
		IsAdmin bool
	}

	type UserKeys2 struct {
		Id int64
	}
	s3 := schemabuilder.NewSchemaWithName("s3")
	s3.Federation().FederatedFieldFunc("User", func(args struct{ Keys []UserKeys2 }) []*UserWithAdminPrivelages {
		users := make([]*UserWithAdminPrivelages, 0, len(args.Keys))
		users = append(users, &UserWithAdminPrivelages{Id: int64(1), IsAdmin: true})
		return users
	})

	user3 := s3.Object("User", UserWithAdminPrivelages{})
	user3.Key("id")
	user3.FieldFunc("privelages", func(ctx context.Context, user *UserWithAdminPrivelages) (string, error) {
		return "all", nil
	})

	// Create the executor with all the schemas
	ctx := context.Background()
	execs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"s1": s1,
		"s2": s2,
		"s3": s3,
	})
	if err != nil {
		return nil, nil, nil, err
	}

	e, err := NewExecutor(ctx, execs)
	return e, s1, s2, err
}

func TestExecutorQueriesFieldsOnMultipleServices2(t *testing.T) {
	e, _, _, err := createExecutorWithFederatedObjects2()
	require.NoError(t, err)
	testCases := []struct {
		Name          string
		Query         string
		Output        string
		Error         bool
		ExpectedError string
	}{
		{
			Name: "query fields with args",
			Query: `
			query Foo {
				users {
					email
				}
			}`,
			Output: `
			{
				"users":[
					{
						"__key":1,
						"email":"yaaayeeeet@gmail.com"
					}
				]
			}`,
			Error: false,
		},
		{
			Name: "query fields with args 2",
			Query: `
			query Foo {
				users {
					secret
					privelages
				}
			}`,
			Output: `
			{
				"users":[
					{
						"__key":1,
						"secret":"shhhhh",
						"privelages": "all"
					}
				]
			}`,
			Error: false,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			ctx := context.Background()
			if !testCase.Error {
				// Validates that we were able to execute the query on multiple
				// schemas and correctly stitch the results back together
				runAndValidateQueryResults(t, ctx, e, testCase.Query, testCase.Output)
			} else {
				runAndValidateQueryError(t, ctx, e, testCase.Query, testCase.Output, testCase.ExpectedError)
			}

		})
	}
}
