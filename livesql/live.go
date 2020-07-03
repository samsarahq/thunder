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

// QueryDependency represents a dependency on SQL query.
type QueryDependency struct {
	Table  string
	Filter sqlgen.Filter
}

func (t *dbTracker) registerDependency(ctx context.Context, schema *sqlgen.Schema, table string, tester sqlgen.Tester, filter sqlgen.Filter) error {
	r := &dbResource{
		table:    table,
		tester:   tester,
		resource: reactive.NewResource(),
	}
	r.resource.Cleanup(func() {
		t.remove(r)
	})

	reactive.AddDependency(ctx, r.resource, QueryDependency{Table: table, Filter: filter})

	t.add(r)
	return nil
}

// LiveDB is a SQL client that supports live updating queries.
// It relies on a reactive.Rerunner being in the context to register changes in the database (which
// are propagated through said rerunner to its clients). Without this rerunner being in the context
// it falls back to non-live (sqlgen) behavior.
// See https://godoc.org/github.com/samsarahq/thunder/reactive for information on reactive.
type LiveDB struct {
	*sqlgen.DB

	tracker *dbTracker
}

// NewLiveDB constructs a new LiveDB
func NewLiveDB(db *sqlgen.DB) *LiveDB {
	return &LiveDB{
		DB:      db,
		tracker: newDbTracker(),
	}
}

type queryCacheKey struct {
	clause string
	args   interface{}
}

// query reactively performs a SelectQuery
func (ldb *LiveDB) query(ctx context.Context, query *sqlgen.BaseSelectQuery) ([]interface{}, error) {
	// Fall back to sqlgen querying if there is no reactive rerunner present or if we're in
	// a transaction.
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
	key := queryCacheKey{clause: clause, args: internal.MakeHashable(args)}

	result, err := reactive.Cache(ctx, key, func(ctx context.Context) (interface{}, error) {
		// Build a tester for the dependency.
		tester, err := ldb.Schema.MakeTester(query.Table.Name, query.Filter)
		if err != nil {
			return nil, err
		}

		// Register the dependency before we do the query to not miss any updates
		// between querying and registering.
		// Do not fail the query if this step fails.
		_ = ldb.tracker.registerDependency(ctx, ldb.Schema, query.Table.Name, tester, query.Filter)

		// Perform the query.
		// XXX: This will build the SQL string again... :(
		return ldb.DB.BaseQuery(ctx, query)
	})

	if err != nil {
		return nil, err
	}
	return result.([]interface{}), nil
}

// Query fetches a collection of rows from the database and will invalidate ctx
// when the query result changes
//
// result should be a pointer to a slice of pointers to structs, for example:
//
//   var users []*User
//   if err := ldb.Query(ctx, &users, nil, nil); err != nil {
//
func (ldb *LiveDB) Query(ctx context.Context, result interface{}, filter sqlgen.Filter, options *sqlgen.SelectOptions) error {
	query, err := ldb.Schema.MakeSelect(result, filter, options)
	if err != nil {
		return err
	}

	rows, err := ldb.query(ctx, query)
	if err != nil {
		return err
	}

	return sqlgen.CopySlice(result, rows)
}

// FullScanQuery bypasses any index checking on a query.
// Normal LiveDB.Query will check during tests if the query uses an index and will fail tests if not. This function
// will skip those checks.
// There are cases where we explicitly want to support full table scans such as
// 1. During tests to verify results (eg get all)
// 2. Some rare operations are infrequent and its better to have no index and instead perform full table scans
//    when that query is run.
func (ldb *LiveDB) FullScanQuery(ctx context.Context, result interface{}, filter sqlgen.Filter, options *sqlgen.SelectOptions) error {
	if options == nil {
		options = &sqlgen.SelectOptions{}
	}
	options.AllowNoIndex = true

	return ldb.Query(ctx, result, filter, options)
}

// QueryRow fetches a single row from the database and will invalidate ctx when
// the query result changes
//
// result should be a pointer to a pointer to a struct, for example:
//
//   var user *User
//   if err := ldb.Query(ctx, &user, Filter{"id": 10}, nil); err != nil {
//
func (ldb *LiveDB) QueryRow(ctx context.Context, result interface{}, filter sqlgen.Filter, options *sqlgen.SelectOptions) error {
	query, err := ldb.Schema.MakeSelectRow(result, filter, options)
	if err != nil {
		return err
	}

	rows, err := ldb.query(ctx, query)
	if err != nil {
		return err
	}

	return sqlgen.CopySingletonSlice(result, rows)
}

func (ldb *LiveDB) Close() error {
	return ldb.Conn.Close()
}

func (ldb *LiveDB) AddDependency(ctx context.Context, dep QueryDependency) error {
	tester, err := ldb.Schema.MakeTester(dep.Table, dep.Filter)
	if err != nil {
		return err
	}

	if err := ldb.tracker.registerDependency(ctx, ldb.Schema, dep.Table, tester, dep.Filter); err != nil {
		return err
	}
	return nil
}
