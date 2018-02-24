package sqlgen

import (
	"context"
	"database/sql"
	"errors"

	"github.com/samsarahq/thunder/batch"
)

// DB uses a *sql.DB connection that is established by its owner. DB assumes the
// database connection exists and is alive at all times during the lifecycle of
// the object.
type DB struct {
	Conn   *sql.DB
	Schema *Schema

	batchFetch *batch.Func
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

func (db *DB) BaseQuery(ctx context.Context, query *BaseSelectQuery) ([]interface{}, error) {
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

	_, err = db.execWithTrace(ctx, query, "DeleteRow")
	return err
}

// txKey is used as a key for a context.Context to hold a transaction.
//
// With multiple open databases, each database can store its own transactions in a context.
type txKey struct {
	db *DB
}

func (db *DB) WithTx(ctx context.Context) (context.Context, *sql.Tx, error) {
	maybeTx := ctx.Value(txKey{db: db})
	if maybeTx != nil {
		return nil, nil, errors.New("already in a tx")
	}

	tx, err := db.Conn.Begin()
	if err != nil {
		return nil, nil, err
	}
	return context.WithValue(ctx, txKey{db: db}, tx), tx, nil
}

func (db *DB) WithExistingTx(ctx context.Context, tx *sql.Tx) (context.Context, error) {
	maybeTx := ctx.Value(txKey{db: db})
	if maybeTx != nil {
		return nil, errors.New("already in a tx")
	}

	return context.WithValue(ctx, txKey{db: db}, tx), nil
}

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
