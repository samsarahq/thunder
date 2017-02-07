package graphql_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/samsarahq/thunder/reactive"
)

type User struct {
	Name     string
	Age      int
	resource *reactive.Resource
}

type Slow struct {
}

func TestPathError(t *testing.T) {
	schema := schemabuilder.NewSchema()

	type Inner struct{}

	query := schema.Query()
	query.FieldFunc("inner", func() Inner {
		return Inner{}
	})

	query.FieldFunc("safe", func() error {
		return graphql.NewSafeError("safe safe")
	})

	_ = schema.Mutation()

	type Expensive struct{}

	inner := schema.Object("inner", Inner{})
	inner.FieldFunc("expensive", func(ctx context.Context) Expensive {
		return Expensive{}
	})
	inner.FieldFunc("inners", func(ctx context.Context) []Inner {
		return []Inner{Inner{}}
	})

	nested := schema.Object("expensive", Expensive{})
	nested.FieldFunc("expensives", func(ctx context.Context) []Expensive {
		return []Expensive{Expensive{}}
	})

	nested.FieldFunc("err", func() error {
		return errors.New("no good, bad")
	})

	builtSchema := schema.MustBuild()

	q := graphql.MustParse(`
		{
			inner { inners { expensive { expensives { err } } } }
        }`, nil)

	if err := graphql.PrepareQuery(builtSchema.Query, q); err != nil {
		t.Error(err)
	}

	e := graphql.Executor{}
	_, err := e.Execute(context.Background(), builtSchema.Query, nil, q)
	if err == nil || err.Error() != "inner.inners.0.expensive.expensives.0.err: no good, bad" {
		t.Errorf("bad error: %v", err)
	}

	q = graphql.MustParse(`
		{
			safe
		}`, nil)

	if err := graphql.PrepareQuery(builtSchema.Query, q); err != nil {
		t.Error(err)
	}

	e = graphql.Executor{}
	_, err = e.Execute(context.Background(), builtSchema.Query, nil, q)
	if err == nil || err.Error() != "safe safe" {
		t.Errorf("bad error: %v", err)
	}
	if _, ok := err.(graphql.SanitizedError); !ok {
		t.Errorf("safe not safe")
	}

}

// TestEndToEndAwaitAndCache tests that slow fields get run in parallel and cached.
//
// The test verifies that the `slow` field on user, which sleeps for 100ms, gets
// run in parallel by verifying the total runtime over several users.
//
// The test verifies that a `count` sub-field of the `slow` field is cached by
// invalidating a single `slow` call, and tracking the number of calls to count.
func TestEndToEndAwaitAndCache(t *testing.T) {
	users := []*User{
		{Name: "Alice", Age: 5, resource: reactive.NewResource()},
		{Name: "Bob", Age: 6, resource: reactive.NewResource()},
		{Name: "Charlie", Age: 7, resource: reactive.NewResource()},
	}

	var mu sync.Mutex
	calls := 0

	schema := schemabuilder.NewSchema()

	query := schema.Query()
	query.FieldFunc("users", func(ctx context.Context) []*User {
		return users
	})

	_ = schema.Mutation()

	user := schema.Object("User", User{})
	user.FieldFunc("slow", func(ctx context.Context, u *User) *Slow {
		reactive.AddDependency(ctx, u.resource)
		time.Sleep(100 * time.Millisecond)
		return new(Slow)
	})

	slow := schema.Object("Slow", Slow{})
	slow.FieldFunc("count", func() bool {
		mu.Lock()
		calls++
		mu.Unlock()
		return true
	})

	builtSchema := schema.MustBuild()

	q := graphql.MustParse(`
		{
			users {
				name
				slow { count }
            }
        }`, nil)

	if err := graphql.PrepareQuery(builtSchema.Query, q); err != nil {
		t.Error(err)
	}

	results := make(chan interface{})

	start := time.Now()
	rerunner := reactive.NewRerunner(context.Background(), func(ctx context.Context) (interface{}, error) {
		e := graphql.Executor{}
		result, err := e.Execute(ctx, builtSchema.Query, nil, q)
		if err != nil {
			t.Error(err)
		}

		results <- asJSON(result)
		return nil, nil
	}, 0)
	defer rerunner.Stop()

	result := <-results
	duration := time.Since(start)
	if duration > 150*time.Millisecond {
		t.Errorf("did not execute in parallel; duration %v > 150ms", duration)
	}
	if !reflect.DeepEqual(result, parseJSON(`
		{"users": [
			{"name": "Alice", "slow": {"count": true}},
			{"name": "Bob", "slow": {"count": true}},
			{"name": "Charlie", "slow": {"count": true}}
        ]}`)) {
		t.Error("bad value")
	}
	if calls != 3 {
		t.Errorf("expected 3 calls to slow, got %d", calls)
	}

	start = time.Now()
	users[0].resource.Strobe()
	result = <-results
	duration = time.Since(start)
	if duration > 150*time.Millisecond {
		t.Errorf("did not execute in parallel; duration %v > 150ms", duration)
	}
	if !reflect.DeepEqual(result, parseJSON(`
		{"users": [
			{"name": "Alice", "slow": {"count": true}},
			{"name": "Bob", "slow": {"count": true}},
			{"name": "Charlie", "slow": {"count": true}}
        ]}`)) {
		t.Error("bad value")
	}
	if calls != 4 {
		t.Errorf("expected 4 total calls to slow, got %d", calls)
	}
}
