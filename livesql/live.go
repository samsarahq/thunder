package livesql

import (
	"context"
	"sync"

	"github.com/samsarahq/thunder/internal"
	"github.com/samsarahq/thunder/reactive"
	"github.com/samsarahq/thunder/sqlgen"
)

// dbResource tracks changes to a specific table matching a filter
type dbResource struct {
	table    string
	tester   sqlgen.Tester
	resource *reactive.Resource
}

func (r *dbResource) shouldInvalidate(update *update) bool {
	// Bail out quickly if the table does not match.
	if r.table != update.table {
		return false
	}

	// If we failed to parse an update we don't know what happened, so we
	// invalidate.
	if update.err != nil {
		return true
	}

	// Invalidate if any of the updated rows match the query.
	for _, d := range update.deltas {
		if r.tester.Test(d.before) || r.tester.Test(d.after) {
			return true
		}
	}
	return false
}

// dbTracker tracks many dbResources
type dbTracker struct {
	mu        sync.Mutex
	resources map[*dbResource]struct{}
}

func newDbTracker() *dbTracker {
	return &dbTracker{
		resources: make(map[*dbResource]struct{}),
	}
}

func (t *dbTracker) add(r *dbResource) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.resources[r] = struct{}{}
}

func (t *dbTracker) remove(r *dbResource) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.resources, r)
}

// processBinlog processes a set of updates from the MySQL binlog
func (t *dbTracker) processBinlog(update *update) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for q := range t.resources {
		if q.shouldInvalidate(update) {
			q.resource.Invalidate()
		}
	}
}

func (t *dbTracker) registerDependency(ctx context.Context, table string, tester sqlgen.Tester) {
	r := &dbResource{
		table:    table,
		tester:   tester,
		resource: reactive.NewResource(),
	}
	r.resource.Cleanup(func() {
		t.remove(r)
	})
	reactive.AddDependency(ctx, r.resource)

	t.add(r)
}

// LiveDB is a SQL client that supports live updating queries
type LiveDB struct {
	*sqlgen.DB

	tracker *dbTracker
}

// NewLiveDB constructs a new LiveDB
func NewLiveDB(db *sqlgen.DB) *LiveDB {
	dbCopy := *db
	ldb := &LiveDB{
		DB:      &dbCopy,
		tracker: newDbTracker(),
	}
	dbCopy.BaseQueryer = ldb
	return ldb
}

type queryCacheKey struct {
	clause string
	args   interface{}
}

func (ldb *LiveDB) BaseQuery(ctx context.Context, query *sqlgen.BaseSelectQuery) ([]interface{}, error) {
	if !reactive.HasRerunner(ctx) || ldb.HasTx(ctx) {
		return ldb.DB.BaseQuery(ctx, query)
	}

	selectQuery, err := query.MakeSelectQuery()
	if err != nil {
		return nil, err
	}

	clause, args := selectQuery.ToSQL()

	// Build a cache key for the query. Convert the args slice into an array so
	// it can be stored as a map key.
	key := queryCacheKey{clause: clause, args: internal.ToArray(args)}

	result, err := reactive.Cache(ctx, key, func(ctx context.Context) (interface{}, error) {
		// Build a tester for the dependency.
		tester, err := ldb.Schema.MakeTester(query.Table.Name, query.Filter)
		if err != nil {
			return nil, err
		}

		// Register the dependency before we do the query to not miss any updates
		// between querying and registering.
		ldb.tracker.registerDependency(ctx, query.Table.Name, tester)

		// Perform the query.
		// XXX: This will build the SQL string again... :(
		return ldb.DB.BaseQuery(ctx, query)
	})

	if err != nil {
		return nil, err
	}
	return result.([]interface{}), nil
}
