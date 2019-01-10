package testgraphql

import (
	"context"
	"testing"

	"github.com/samsarahq/go/snapshotter"
	"github.com/samsarahq/thunder/graphql"
	"github.com/stretchr/testify/require"
)

type Snapshotter struct {
	*snapshotter.Snapshotter
	t        *testing.T
	executor graphql.Executor
	schema   *graphql.Schema
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
	err := graphql.PrepareQuery(s.schema.Query, q.SelectionSet)
	if err != nil && opt.recordError {
		s.Snapshot(name, struct{ Error string }{err.Error()})
		return
	}
	require.NoError(s.t, err)
	output, err := s.executor.Execute(context.Background(), s.schema.Query, nil, q)

	if err != nil && opt.recordError {
		s.Snapshot(name, struct{ Error string }{err.Error()})
	} else {
		require.NoError(s.t, err)
		s.Snapshot(name, output)
	}
}
