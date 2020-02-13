package federation


import (
	"context"

	"testing"


	"github.com/samsarahq/thunder/graphql/introspection"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
)

func createProxySchemas() (map[string][]byte, error) {
	// The first schema has a user object with an id and orgId
	type User struct {
		Id    int64
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
		users = append(users, &User{Id: int64(1), OrgId: int64(9086)})
		return users, nil
	})

	type Admin struct {
		Id         int64
		OrgId      int64
		SuperPower string
	}
	admin := s1.Object("Admin", Admin{})
	admin.Key("id")
	admin.Federation(func(a *Admin) int64 {
		return a.Id
	})
	s1.Query().FieldFunc("admins", func(ctx context.Context) ([]*Admin, error) {
		admins := make([]*Admin, 0, 1)
		admins = append(admins, &Admin{Id: int64(1), OrgId: int64(9086), SuperPower: "flying"})
		return admins, nil
	})

	type Everyone struct {
		schemabuilder.Union
		*User
		*Admin
	}
	s1.Query().FieldFunc("everyone", func(ctx context.Context) ([]*Everyone, error) {
		everyone := make([]*Everyone, 0, 2)
		everyone = append(everyone, &Everyone{Admin: &Admin{Id: int64(1), OrgId: int64(9086), SuperPower: "flying"}})
		everyone = append(everyone, &Everyone{User: &User{Id: int64(2), OrgId: int64(9086)}})
		return everyone, nil
	})

	// The second schema has a user with an email and a secret field
	type UserWithEmail struct {
		Id    int64
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

	// ctx := context.Background()

	schemaJson1, err := introspection.ComputeSchemaJSON(*s1)
	if err != nil {
		return nil, err
	}
	schemaJson2, err := introspection.ComputeSchemaJSON(*s2)
	if err != nil {
		return nil, err
	}

	schemas := make(map[string][]byte)
	schemas["s1"] = schemaJson1
	schemas["s2"] = schemaJson2
	return schemas, nil
}


func TestProxy(t *testing.T) {
	createProxySchemas()

	p := &Proxy{}
	ctx := context.Background()

	p.Poll(ctx)
}