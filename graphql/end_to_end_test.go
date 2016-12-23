package graphql_test

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/samsarahq/thunder/reactive"
)

type user struct {
	Name     string
	Age      int
	resource *reactive.Resource
}

type slow struct {
}

type schema struct {
	users []*user

	mu    sync.Mutex
	calls int
}

func (s *schema) Query() schemabuilder.Object {
	object := schemabuilder.Object{}

	object.FieldFunc("users", func(ctx context.Context) []*user {
		return s.users
	})

	return object
}

func (s *schema) Mutation() schemabuilder.Object {
	return schemabuilder.Object{}
}

func (s *schema) User() schemabuilder.Object {
	object := schemabuilder.Object{
		Type: user{},
	}

	object.FieldFunc("slow", func(ctx context.Context, u *user) *slow {
		reactive.AddDependency(ctx, u.resource)
		time.Sleep(100 * time.Millisecond)
		return new(slow)
	})

	return object
}

func (s *schema) Slow() schemabuilder.Object {
	object := schemabuilder.Object{
		Type: slow{},
	}

	object.FieldFunc("count", func() bool {
		s.mu.Lock()
		s.calls++
		s.mu.Unlock()
		return true
	})

	return object

}

// TestEndToEndAwaitAndCache tests that slow fields get run in parallel and cached.
//
// The test verifies that the `slow` field on user, which sleeps for 100ms, gets
// run in parallel by verifying the total runtime over several users.
//
// The test verifies that a `count` sub-field of the `slow` field is cached by
// invalidating a single `slow` call, and tracking the number of calls to count.
func TestEndToEndAwaitAndCache(t *testing.T) {
	schema := &schema{
		users: []*user{
			{Name: "Alice", Age: 5, resource: reactive.NewResource()},
			{Name: "Bob", Age: 6, resource: reactive.NewResource()},
			{Name: "Charlie", Age: 7, resource: reactive.NewResource()},
		},
	}

	builtSchema := schemabuilder.MustBuildSchema(schema)

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
		e := graphql.Executor{MaxConcurrency: 1}
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
	if schema.calls != 3 {
		t.Errorf("expected 3 calls to slow, got %d", schema.calls)
	}

	start = time.Now()
	schema.users[0].resource.Strobe()
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
	if schema.calls != 4 {
		t.Errorf("expected 4 total calls to slow, got %d", schema.calls)
	}
}
