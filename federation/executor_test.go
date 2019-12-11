package federation

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/samsarahq/thunder/reactive"
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

func assertExecuteEqual(ctx context.Context, t *testing.T, e *Executor, in, out string) {
	res, err := e.Execute(ctx, graphql.MustParse(in, map[string]interface{}{}))
	require.NoError(t, err)

	var expected interface{}
	err = json.Unmarshal([]byte(out), &expected)
	require.NoError(t, err)

	assert.Equal(t, expected, roundtripJson(t, res))
}

func assertExecuteError(ctx context.Context, t *testing.T, e *Executor, in, errMsg string) {
	_, err := e.Execute(ctx, graphql.MustParse(in, map[string]interface{}{}))
	require.EqualError(t, err, errMsg)
}

// TestExecutorOneMutate tests that a single mutate is supported.
func TestExecutorOneMutate(t *testing.T) {
	s1 := schemabuilder.NewSchema()
	s1.Mutation().FieldFunc("orderMatters", func(ctx context.Context) (string, error) {
		return "ok", nil
	})

	ctx := context.Background()
	execs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"s1": s1,
	})
	require.NoError(t, err)

	e, err := NewExecutor(ctx, execs)
	require.NoError(t, err)
	assertExecuteEqual(ctx, t, e, `
		mutation Foo {
			orderMatters
		}
	`, `
		{"orderMatters": "ok"}
	`)
}

// TestExecutorTwoMutates tests that two mutates fail.
func TestExecutorTwoMutates(t *testing.T) {
	s1 := schemabuilder.NewSchema()
	s1.Mutation().FieldFunc("s1", func(ctx context.Context) (string, error) {
		return "ok", nil
	})
	s2 := schemabuilder.NewSchema()
	s2.Mutation().FieldFunc("s2", func(ctx context.Context) (string, error) {
		return "ok", nil
	})

	ctx := context.Background()
	execs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"s1": s1,
		"s2": s2,
	})
	require.NoError(t, err)

	e, err := NewExecutor(ctx, execs)
	require.NoError(t, err)
	assertExecuteError(ctx, t, e, `
		mutation Foo {
			s1
			s2
		}
	`, "only support 1 mutation step to maintain ordering")
}

// TestExecutorHasReactiveCache tests that a reactive.Cache works.
func TestExecutorHasReactiveCache(t *testing.T) {
	schema := schemabuilder.NewSchema()
	schema.Query().FieldFunc("testCache", func(ctx context.Context) (string, error) {
		count := 0
		for i := 0; i < 2; i++ {
			v, err := reactive.Cache(ctx, "", func(ctx context.Context) (interface{}, error) {
				count++
				return "", nil
			})
			assert.NoError(t, err)
			assert.Equal(t, v, "")
		}
		assert.Equal(t, 1, count)
		return "", nil
	})

	ctx := context.Background()

	execs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"schema": schema,
	})
	require.NoError(t, err)

	e, err := NewExecutor(ctx, execs)
	require.NoError(t, err)
	assertExecuteEqual(ctx, t, e, `
		{
			testCache
		}
	`, `
		{"testCache": ""}
	`)
}

// TestExecutorCancelsLeakedContext tests that a context kept around will be canceled.
func TestExecutorCancelsLeakedContext(t *testing.T) {
	var stashedCtx context.Context

	schema := schemabuilder.NewSchema()
	schema.Query().FieldFunc("stash", func(ctx context.Context) (string, error) {
		stashedCtx = ctx
		return "", nil
	})

	ctx := context.Background()

	execs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"schema": schema,
	})
	require.NoError(t, err)

	e, err := NewExecutor(ctx, execs)
	require.NoError(t, err)
	assertExecuteEqual(ctx, t, e, `
		{
			stash
		}
	`, `
		{"stash": ""}
	`)

	<-stashedCtx.Done()
	assert.EqualError(t, stashedCtx.Err(), "context canceled")
}

// TestExecutorReturnsFailure tests that a sub-query error gets bubbled up.
func TestExecutorReturnsFailure(t *testing.T) {
	schema := schemabuilder.NewSchema()
	schema.Query().FieldFunc("fail", func(ctx context.Context) (string, error) {
		return "", errors.New("uh oh")
	})

	ctx := context.Background()

	execs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"schema": schema,
	})
	require.NoError(t, err)

	e, err := NewExecutor(ctx, execs)
	require.NoError(t, err)
	assertExecuteError(ctx, t, e, `
		{
			fail
		}
	`, "executing sub plan: run on service: execute remotely: executing query: fail: uh oh")
}

// TestExecutorCancelsOnFailure tests that a failing sub-query cancels
// other in-flight sub-queries.
func TestExecutorCancelsOnFailure(t *testing.T) {
	// s1 will fail, and expect s2 to be canceled in turn.
	s2started := make(chan struct{}, 0)
	s2canceled := make(chan struct{}, 0)

	s1 := schemabuilder.NewSchema()
	s1.Query().FieldFunc("fail", func(ctx context.Context) (string, error) {
		<-s2started
		return "", errors.New("fail")
	})

	s2 := schemabuilder.NewSchema()
	s2.Query().FieldFunc("wait", func(ctx context.Context) (string, error) {
		close(s2started)
		<-ctx.Done()
		assert.EqualError(t, ctx.Err(), "context canceled")
		close(s2canceled)
		return "", ctx.Err()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	execs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"s1": s1,
		"s2": s2,
	})
	require.NoError(t, err)

	e, err := NewExecutor(ctx, execs)
	require.NoError(t, err)
	assertExecuteError(ctx, t, e, `
		{
			fail
			wait
		}
	`, "executing sub plan: run on service: execute remotely: executing query: fail: fail")

	// Make sure s2 was actually canceled.
	<-s2canceled
}

// TestExecutorCanBeCanceled tests that a canceled request at the top
// cancels in-flight sub-queries.
//
// This should capture the case where top request times out.
//
// XXX: test that no new requests get started ???
func TestExecutorCanBeCanceled(t *testing.T) {
	// Once the the request is started it will be canceled.
	started := make(chan struct{}, 0)
	canceled := make(chan struct{}, 0)

	schema := schemabuilder.NewSchema()
	schema.Query().FieldFunc("hang", func(ctx context.Context) (string, error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		return "", ctx.Err()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	execs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"schema": schema,
	})
	require.NoError(t, err)

	go func() {
		<-started
		cancel()
	}()

	e, err := NewExecutor(ctx, execs)
	require.NoError(t, err)
	assertExecuteError(ctx, t, e, `
		{
			hang
		}
	`, "executing sub plan: run on service: execute remotely: executing query: hang: context canceled")

	// Make sure the request was actually canceled.
	<-canceled
}

// TestExecutorRunsPlanInParallel tests that plans that can run concurrently do run
// concurrently.
//
// It's not a very exhaustive test.
func TestExecutorRunsPlanInParallel(t *testing.T) {
	type Foo struct {
	}

	var running chan chan struct{}

	makeSchema := func(prefix string) *schemabuilder.Schema {
		schema := schemabuilder.NewSchema()
		schema.Query().FieldFunc(prefix+"foo", func() *Foo {
			return &Foo{}
		})

		foo := schema.Object("Foo", Foo{})
		foo.Federation(func(*Foo) string {
			return ""
		})
		foo.FieldFunc(prefix+"instant", func(f *Foo) *Foo {
			return f
		})
		foo.FieldFunc(prefix+"wait", func(f *Foo) *Foo {
			// xxx: store path in Foo by concatenating field name funcs and then
			// use that to record exec order?
			release := <-running
			<-release
			return f
		})
		foo.FieldFunc(prefix+"echo", func(f *Foo) string {
			return "echo"
		})

		schema.Federation().FieldFunc("Foo", func(args struct{ Keys []string }) []*Foo {
			foos := make([]*Foo, 0, len(args.Keys))
			for range args.Keys {
				foos = append(foos, &Foo{})
			}
			return foos
		})

		return schema
	}

	ctx := context.Background()

	// todo: assert specific invocation traces?

	execs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"schema1": makeSchema("s1"),
		"schema2": makeSchema("s2"),
	})
	require.NoError(t, err)

	e, err := NewExecutor(ctx, execs)
	require.NoError(t, err)
	require.NoError(t, err)

	running = make(chan chan struct{}, 0)
	go func() {
		release := make(chan struct{}, 0)
		for i := 0; i < 3; i++ {
			running <- release
		}
		close(release)
	}()

	assertExecuteEqual(ctx, t, e, `
		{
			s1foo {
				s1instant {
					s2wait {
						s2echo
					}
				}
				s2instant {
					s1wait {
						s1echo
					}
				}
			}
			s2foo {
				s1wait {
					s1echo
				}
			}
		}
	`, `
		{
			"s1foo": {
				"s1instant": {"s2wait": {"s2echo": "echo"}},
				"s2instant": {"s1wait": {"s1echo": "echo"}}
			},
			"s2foo": {
				"s1wait": {"s1echo": "echo"}
			}
		}
	`)

	// xxx: test and verify concurrency limit?
}
