package livesql

import (
	"context"
	"errors"
	"sync"

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
	if ldb.HasTx(ctx) {
		return nil, errors.New("can't use both tx and rerunner")
	}

	selectQuery, err := query.MakeSelectQuery()
	if err != nil {
		return nil, err
	}

	clause, args := selectQuery.ToSQL()

	// Build a cache key for the query. Convert the args slice into an array so
	// it can be stored as a map key.
	key := queryCacheKey{clause: clause, args: toArray(args)}

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
		res, err := ldb.Conn.Query(clause, args...)
		if err != nil {
			return nil, err
		}
		defer res.Close()
		return ldb.Schema.ParseRows(selectQuery, res)
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
	if !reactive.HasRerunner(ctx) || ldb.HasTx(ctx) {
		return ldb.DB.Query(ctx, result, filter, options)
	}

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

// QueryRow fetches a single row from the database and will invalidate ctx when
// the query result changes
//
// result should be a pointer to a pointer to a struct, for example:
//
//   var user *User
//   if err := ldb.Query(ctx, &user, Filter{"id": 10}, nil); err != nil {
//
func (ldb *LiveDB) QueryRow(ctx context.Context, result interface{}, filter sqlgen.Filter, options *sqlgen.SelectOptions) error {
	if !reactive.HasRerunner(ctx) || ldb.HasTx(ctx) {
		return ldb.DB.QueryRow(ctx, result, filter, options)
	}

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
