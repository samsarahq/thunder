package graphql_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/samsarahq/thunder/reactive"
	"github.com/stretchr/testify/require"
)

func TestReactiveCacheResetsOnError(t *testing.T) {
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
	query.FieldFunc("uncachedError", func() (string, error) {
		return "", errors.New("this is not cached")
	})
	_ = schema.Mutation()

	user := schema.Object("User", User{})
	user.FieldFunc("slow", func(ctx context.Context, u *User) *Slow {
		reactive.AddDependency(ctx, u.resource, nil)
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
			uncachedError
		}`, nil)

	runGrabber := newConcurrencyManager(t, 1)
	runGrabber.Inc()

	runFinish := newConcurrencyManager(t, 1)

	rerunner := reactive.NewRerunner(context.Background(), func(ctx context.Context) (interface{}, error) {
		if !runGrabber.Dec() {
			return nil, nil
		}
		e := graphql.NewExecutor(graphql.NewImmediateGoroutineScheduler())
		_, err := e.Execute(ctx, builtSchema.Query, nil, q)
		require.Error(t, err)

		require.True(t, runFinish.Inc(), "could not push finish channel")

		return nil, reactive.RetrySentinelError
	}, 0, false)
	defer rerunner.Stop()

	require.True(t, runFinish.Dec(), "error waiting for run to finish")
	if calls != 3 {
		t.Errorf("expected 3 calls to slow, got %d", calls)
	}

	runGrabber.Inc()

	require.True(t, runFinish.Dec(), "error waiting for run to finish")
	if calls != 6 {
		t.Errorf("expected 6 total calls to slow, got %d", calls)
	}
	runFinish.Close()
	runGrabber.Close()
}

func newConcurrencyManager(t *testing.T, size int) *concurrencyManager {
	return &concurrencyManager{
		t:           t,
		done:        make(chan struct{}),
		counterChan: make(chan struct{}, size),
	}
}

type concurrencyManager struct {
	t           *testing.T
	done        chan struct{}
	counterChan chan struct{}
}

func (c *concurrencyManager) Inc() (ok bool) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	select {
	case <-ctx.Done():
		require.Fail(c.t, "test timed out", ctx.Err())
		return false
	case <-c.done:
		return false
	case c.counterChan <- struct{}{}:
		return true
	}
}

func (c *concurrencyManager) Dec() (ok bool) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	select {
	case <-ctx.Done():
		require.Fail(c.t, "test timed out", ctx.Err().Error())
		return false
	case <-c.done:
		return false
	case <-c.counterChan:
		return true
	}
}

func (c *concurrencyManager) Close() {
	close(c.done)
}
