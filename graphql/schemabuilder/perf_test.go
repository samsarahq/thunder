package schemabuilder

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/reactive"
)

type perfUser struct {
	Name string
	age  int
}

type perfRoot struct {
}

type perfSchema struct{}

func (s *perfSchema) User() Object {
	object := Object{
		Type: perfUser{},
	}
	object.FieldFunc("age", func(u *perfUser) int {
		return u.age
	})
	return object
}

func (s *perfSchema) Query() Object {
	object := Object{
		Type: perfRoot{},
	}
	object.FieldFunc("users", func() []*perfUser {
		users := make([]*perfUser, 5000)
		for i := range users {
			users[i] = &perfUser{
				Name: "user" + fmt.Sprint(i),
				age:  i,
			}
		}
		return users
	})
	return object
}

type perfEmpty struct{}

func (s *perfSchema) Mutation() Object {
	return Object{
		Type: perfEmpty{},
	}
}

func BenchmarkSimpleExecute(b *testing.B) {
	builtSchema := MustBuildSchema(&perfSchema{})
	ctx := context.Background()

	q := graphql.MustParse(`
		{
			users {
				name
				age
			}
		}
	`, nil)

	if err := graphql.PrepareQuery(builtSchema.Query, q); err != nil {
		b.Error(err)
	}

	for i := 0; i < b.N; i++ {
		done := make(chan struct{}, 0)
		reactive.NewRerunner(ctx, func(ctx context.Context) (interface{}, error) {
			e := graphql.Executor{MaxConcurrency: 1}

			_, err := e.Execute(ctx, builtSchema.Query, perfRoot{}, q)
			if err != nil {
				b.Error(err)
			}
			close(done)

			return nil, errors.New("stop")
		}, 0)
		<-done
	}
}
