package sqlgen

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/samsarahq/thunder/batch"
)

// DB uses a *sql.DB connection that is established by its owner. DB assumes the
// database connection exists and is alive at all times during the lifecycle of
// the object.
type DB struct {
	Conn   *sql.DB
	Schema *Schema

	batchFetch *batch.Func
	shardLimit Filter
}

func NewDB(conn *sql.DB, schema *Schema) *DB {
	db := &DB{
		Conn:   conn,
		Schema: schema,
	}

	db.batchFetch = &batch.Func{
		Many: func(ctx context.Context, items []interface{}) ([]interface{}, error) {
			table := items[0].(*BaseSelectQuery).Table

			// First, build the SQL query.
			filters := make([]Filter, 0, len(items))
			for _, item := range items {
				filters = append(filters, item.(*BaseSelectQuery).Filter)
			}
			clause, args := makeBatchQuery(filters)
			query, err := db.Schema.makeSelect(table.Type, nil, &SelectOptions{
				Where:  clause,
				Values: args,
			})
			if err != nil {
				return nil, err
			}
			selectQuery, err := query.MakeSelectQuery()
			if err != nil {
				return nil, err
			}
			clause, args = selectQuery.ToSQL()

			// Then, run the SQL query.
			res, err := db.Conn.QueryContext(ctx, clause, args...)
			if err != nil {
				return nil, err
			}
			defer res.Close()
			rows, err := db.Schema.ParseRows(selectQuery, res)
			if err != nil {
				return nil, err
			}

			// Finally, match the returned rows against the queries.
			matcher := newMatcher()
			for i, item := range items {
				query := item.(*BaseSelectQuery)
				// XXX: This needs more rigor, and a test. For now, call coerceMap on rows
				// and filters to flatten out all pointers to values, etc., to copy what
				// the row tester does when matching against the binlog. This way, a filter
				// specifying age=48 will match a value *age=48.
				matcher.add(i, coerceMap(query.Filter))
			}
			results := make([][]interface{}, len(items))
			for _, row := range rows {
				f := coerceMap(table.extractRow(row))
				for _, idx := range matcher.match(f) {
					i := idx.(int)
					results[i] = append(results[i], row)
				}
			}

			// Convert the [][]interface{} return type into a []interface{} to satisfy
			// the Batch interface.
			rawResults := make([]interface{}, 0, len(items))
			for _, result := range results {
				rawResults = append(rawResults, result)
			}
			return rawResults, nil
		},
		Shard: func(item interface{}) interface{} {
			return item.(*BaseSelectQuery).Table
		},
	}
	return db
}

// WithShardLimit scopes the DB to only allow queries with the given key-value
// pairs. This means any query must include a filter for the key-value pairs in
// the limit, and any write must have columns including the specified key-value
// pairs.
func (db *DB) WithShardLimit(shardLimit Filter) (*DB, error) {
	if db.shardLimit != nil {
		return nil, errors.New("already limited")
	}

	dbCopy := *db
	dbCopy.shardLimit = shardLimit
	return &dbCopy, nil
}

func (db *DB) checkFilterAgainstShardLimit(filter Filter) error {
	if db.shardLimit == nil {
		return nil
	}
	for k, v := range db.shardLimit {
		filterV, ok := filter[k]
		if !ok {
			return fmt.Errorf("db is sharded to require %s = %v, but query does not filter on %s", k, v, k)
		}
		if filterV != v {
			return fmt.Errorf("db is sharded to require %s = %v, but query specifies %s = %v", k, v, k, filterV)
		}
	}
	return nil
}

func (db *DB) checkColumnValuesAgainstShardLimit(columns []string, values []interface{}) error {
	if db.shardLimit == nil {
		return nil
	}
	for k, v := range db.shardLimit {
		var valuesV interface{}
		var ok bool
		for i := range columns {
			if columns[i] == k {
				valuesV = values[i]
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("db is sharded to require %s = %v, but query does not include %s", k, v, k)
		}
		if valuesV != v {
			return fmt.Errorf("db is sharded to require %s = %v, but query has %s = %v", k, v, k, valuesV)
		}
	}
	return nil
}

func (db *DB) BaseQuery(ctx context.Context, query *BaseSelectQuery) ([]interface{}, error) {
	if err := db.checkFilterAgainstShardLimit(query.Filter); err != nil {
		return nil, err
	}

	if query.Options == nil && !db.HasTx(ctx) && batch.HasBatching(ctx) {
		rows, err := db.batchFetch.Invoke(ctx, query)
		if err != nil {
			return nil, err
		}
		return rows.([]interface{}), nil
	}

	selectQuery, err := query.MakeSelectQuery()
	if err != nil {
		return nil, err
	}

	clause, args := selectQuery.ToSQL()

	res, err := db.QueryExecer(ctx).QueryContext(ctx, clause, args...)
	if err != nil {
		return nil, err
	}
	defer res.Close()

	return db.Schema.ParseRows(selectQuery, res)
}

func (db *DB) execWithTrace(ctx context.Context, query SQLQuery, operationName string) (sql.Result, error) {
	clause, args := query.ToSQL()

	return db.QueryExecer(ctx).ExecContext(ctx, clause, args...)
}

// Count counts the number of relevant rows in a database, matching options in filter
//
// model should be a pointer to a struct, for example:
//
//   count, err := db.Count(ctx, &User{}, &res, Filter{})
//   if err != nil { ... }
//
func (db *DB) Count(ctx context.Context, model interface{}, filter Filter) (int64, error) {
	if err := db.checkFilterAgainstShardLimit(filter); err != nil {
		return 0, err
	}

	query, err := db.Schema.makeCount(model, filter)
	if err != nil {
		return 0, err
	}

	countQuery, err := query.makeCountQuery()
	if err != nil {
		return 0, err
	}

	clause, args := countQuery.ToSQL()

	var count int64
	err = db.QueryExecer(ctx).QueryRowContext(ctx, clause, args...).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

// Query fetches a collection of rows from the database
//
// result should be a pointer to a slice of pointers to structs, for example:
//
//   var users []*User
//   if err := db.Query(ctx, &users, nil, nil); err != nil {
//
func (db *DB) Query(ctx context.Context, result interface{}, filter Filter, options *SelectOptions) error {
	query, err := db.Schema.MakeSelect(result, filter, options)
	if err != nil {
		return err
	}

	rows, err := db.BaseQuery(ctx, query)
	if err != nil {
		return err
	}

	return CopySlice(result, rows)
}

// QueryRow fetches a single row from the database
//
// result should be a pointer to a pointer to a struct, for example:
//
//   var user *User
//   if err := db.Query(ctx, &user, Filter{"id": 10}, nil); err != nil {
//
func (db *DB) QueryRow(ctx context.Context, result interface{}, filter Filter, options *SelectOptions) error {
	query, err := db.Schema.MakeSelectRow(result, filter, options)
	if err != nil {
		return err
	}

	rows, err := db.BaseQuery(ctx, query)
	if err != nil {
		return err
	}

	return CopySingletonSlice(result, rows)
}

// InsertRow inserts a single row into the database
//
// row should be a pointer to a struct, for example:
//
//   user := &User{Name: "foo"}
//   if err := db.InsertRow(ctx, user); err != nil {
//
func (db *DB) InsertRow(ctx context.Context, row interface{}) (sql.Result, error) {
	query, err := db.Schema.MakeInsertRow(row)
	if err != nil {
		return nil, err
	}

	if err := db.checkColumnValuesAgainstShardLimit(query.Columns, query.Values); err != nil {
		return nil, err
	}

	return db.execWithTrace(ctx, query, "InsertRow")
}

// UpsertRow inserts a single row into the database
//
// row should be a pointer to a struct, for example:
//
//   user := &User{Name: "foo"}
//   if err := db.UpsertRow(ctx, user); err != nil {
//
func (db *DB) UpsertRow(ctx context.Context, row interface{}) (sql.Result, error) {
	query, err := db.Schema.MakeUpsertRow(row)
	if err != nil {
		return nil, err
	}

	if err := db.checkColumnValuesAgainstShardLimit(query.Columns, query.Values); err != nil {
		return nil, err
	}

	return db.execWithTrace(ctx, query, "UpsertRow")
}

// UpdateRow updates a single row in the database, identified by the row's primary key
//
// row should be a pointer to a struct, for example:
//
//   user := &User{Id; 10, Name: "bar"}
//   if err := db.UpdateRow(ctx, user); err != nil {
//
func (db *DB) UpdateRow(ctx context.Context, row interface{}) error {
	query, err := db.Schema.MakeUpdateRow(row)
	if err != nil {
		return err
	}

	if err := db.checkColumnValuesAgainstShardLimit(
		append(query.Where.Columns, query.Columns...),
		append(query.Where.Values, query.Values...)); err != nil {
		return err
	}

	_, err = db.execWithTrace(ctx, query, "UpsertRow")
	return err
}

// DeleteRow deletes a single row from the database, identified by the row's primary key
//
// row should be a pointer to a struct, for example:
//
//   user := &User{Id; 10}
//   if err := db.DeleteRow(ctx, user); err != nil {
//
func (db *DB) DeleteRow(ctx context.Context, row interface{}) error {
	query, err := db.Schema.MakeDeleteRow(row)
	if err != nil {
		return err
	}

	if err := db.checkColumnValuesAgainstShardLimit(query.Where.Columns, query.Where.Values); err != nil {
		return err
	}

	_, err = db.execWithTrace(ctx, query, "DeleteRow")
	return err
}

// txKey is used as a key for a context.Context to hold a transaction.
//
// With multiple open databases, each database can store its own transactions in a context.
type txKey struct {
	db *DB
}

// WithTx begins a transaction and returns a derived Context that contains
// that transaction. It also returns the transaction value itself, for the
// caller to manipulate (e.g., Commit).
// It is an error to invoke this method on a Context that already contains
// a transaction for this DB.
// On error WithTx returns a non-nil Context, so that the caller can
// still easily use its Context (e.g., to log the error).
func (db *DB) WithTx(ctx context.Context) (context.Context, *sql.Tx, error) {
	maybeTx := ctx.Value(txKey{db: db})
	if maybeTx != nil {
		return ctx, nil, errors.New("already in a tx")
	}

	tx, err := db.Conn.BeginTx(ctx, nil)
	if err != nil {
		return ctx, nil, err
	}
	return context.WithValue(ctx, txKey{db: db}, tx), tx, nil
}

// WithExistingTx returns a derived Context that contains the provided Tx.
// It is an error to invoke this method on a Context that already contains
// a transaction for this DB.
// On error WithExistingTx returns a non-nil Context, so that the caller can
// still easily use its Context (e.g., to log the error).
func (db *DB) WithExistingTx(ctx context.Context, tx *sql.Tx) (context.Context, error) {
	maybeTx := ctx.Value(txKey{db: db})
	if maybeTx != nil {
		return ctx, errors.New("already in a tx")
	}

	return context.WithValue(ctx, txKey{db: db}, tx), nil
}

// HasTx returns whether the provided Context contains a transaction for
// this DB.
func (db *DB) HasTx(ctx context.Context) bool {
	return ctx.Value(txKey{db: db}) != nil
}

// A QueryExecer is either a *sql.Tx or a *sql.DB.
type QueryExecer interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
	Exec(query string, args ...interface{}) (sql.Result, error)

	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

func (db *DB) QueryExecer(ctx context.Context) QueryExecer {
	maybeTx := ctx.Value(txKey{db: db})
	if maybeTx != nil {
		return maybeTx.(*sql.Tx)
	}
	return db.Conn
}

var _ QueryExecer = &sql.Tx{}
var _ QueryExecer = &sql.DB{}
