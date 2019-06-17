package testgraphql

import (
	"context"
	"reflect"
	"testing"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/internal"
	"github.com/stretchr/testify/require"
)

type ExecutorAndName struct {
	Name     string
	Executor graphql.ExecutorRunner
}

func GetExecutors() []ExecutorAndName {
	return []ExecutorAndName{
		{
			Name:     "batchExecutor:",
			Executor: graphql.NewExecutor(graphql.NewImmediateGoroutineScheduler()),
		},
	}
}

func NewExecutorWrapper(t *testing.T) *ExecutorWrapper {
	return &ExecutorWrapper{
		t:          t,
		exactError: true,
	}
}

func NewExecutorWrapperWithoutExactErrorMatch(t *testing.T) *ExecutorWrapper {
	return &ExecutorWrapper{
		t:          t,
		exactError: false,
	}
}

type ExecutorWrapper struct {
	t          *testing.T
	exactError bool
}

func (e *ExecutorWrapper) Execute(ctx context.Context, typ graphql.Type, source interface{}, query *graphql.Query) (interface{}, error) {
	var lastOutput interface{}
	var lastErr error
	runOnce := false
	for _, executorAndName := range GetExecutors() {
		output, err := executorAndName.Executor.Execute(ctx, typ, source, query)
		if !runOnce {
			lastOutput = output
			lastErr = err
			runOnce = true
			continue
		}
		if err != nil {
			require.NotNil(e.t, lastErr)
			if e.exactError {
				require.Equal(e.t, lastErr.Error(), err.Error())
			}
			continue
		}
		require.Nil(e.t, lastErr)
		require.True(
			e.t,
			reflect.DeepEqual(internal.AsJSON(lastOutput), internal.AsJSON(output)),
			"queries for %q do no match between different executors", query.Name,
		)
	}
	return lastOutput, lastErr
}
