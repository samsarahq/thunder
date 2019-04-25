package schemabuilder

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/reactive"
)

func BenchmarkSimpleExecute(b *testing.B) {
	schema := NewSchema()

	query := schema.Query()
	query.FieldFunc("users", func() []*User {
		users := make([]*User, 5000)
		for i := range users {
			users[i] = &User{
				Name: "user" + fmt.Sprint(i),
				Age:  i,
			}
		}
		return users
	})

	_ = schema.Mutation()

	builtSchema := schema.MustBuild()
	ctx := context.Background()

	q := graphql.MustParse(`
		{
			users {
				name
				age
			}
		}
	`, nil)

	if err := graphql.PrepareQuery(builtSchema.Query, q.SelectionSet); err != nil {
		b.Error(err)
	}

	for i := 0; i < b.N; i++ {
		done := make(chan struct{}, 0)
		reactive.NewRerunner(ctx, func(ctx context.Context) (interface{}, error) {
			e := graphql.Executor{}

			_, err := e.Execute(ctx, builtSchema.Query, nil, q)
			if err != nil {
				b.Error(err)
			}
			close(done)

			return nil, errors.New("stop")
		}, 0, false)
		<-done
	}
}
