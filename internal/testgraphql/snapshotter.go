package testgraphql

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/samsarahq/go/snapshotter"
	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/internal"
	"github.com/stretchr/testify/require"
)

type Snapshotter struct {
	*snapshotter.Snapshotter
	t      *testing.T
	schema *graphql.Schema
}

func NewSnapshotter(t *testing.T, schema *graphql.Schema) *Snapshotter {
	return &Snapshotter{
		t:           t,
		Snapshotter: snapshotter.New(t),
		schema:      schema,
	}
}

type Option func(*instanceOption)

// Don't fail on error, snapshot it instead.
func RecordError(o *instanceOption) { o.recordError = true }

type instanceOption struct{ recordError bool }

func applyOpts(opts []Option) instanceOption {
	var option instanceOption
	for _, opt := range opts {
		opt(&option)
	}
	return option
}

func (s *Snapshotter) SnapshotQuery(name, query string, opts ...Option) {
	opt := applyOpts(opts)

	q := graphql.MustParse(query, nil)

	var lastOutput interface{}
	var lastErr error
	runOnce := false
	for _, executorAndName := range GetExecutors() {
		output, err := executorAndName.Executor.Execute(context.Background(), s.schema.Query, q)

		if err != nil && opt.recordError {
			s.Snapshot(fmt.Sprintf("%s%s", executorAndName.Name, name), struct{ Error string }{err.Error()})
		} else {
			require.NoError(s.t, err)
			s.Snapshot(fmt.Sprintf("%s%s", executorAndName.Name, name), output)
		}
		if !runOnce {
			lastOutput = output
			lastErr = err
			runOnce = true
			continue
		}
		if err != nil {
			require.NotNil(s.t, lastErr)
			require.Equal(s.t, lastErr.Error(), err.Error())
			continue
		}
		require.Nil(s.t, lastErr)
		require.True(
			s.t,
			reflect.DeepEqual(internal.AsJSON(lastOutput), internal.AsJSON(output)),
			"Snapshots for %q do no match between different executors", name,
		)
	}
}
