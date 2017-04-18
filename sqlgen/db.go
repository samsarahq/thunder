package sqlgen

import (
	"context"
	"database/sql"
	"errors"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
)

// DB uses a *sql.DB connection that is established by its owner. DB assumes the
// database connection exists and is alive at all times during the lifecycle of
// the object.
type DB struct {
	Conn   *sql.DB
	Schema *Schema
}

func NewDB(conn *sql.DB, schema *Schema) *DB {
	return &DB{
		Conn:   conn,
		Schema: schema,
	}
}

func (db *DB) query(ctx context.Context, query *BaseSelectQuery) ([]interface{}, error) {
	selectQuery, err := query.MakeSelectQuery()
	if err != nil {
		return nil, err
	}

	clause, fields := selectQuery.ToSQL()

	if span := opentracing.SpanFromContext(ctx); span != nil {
		span, ctx = opentracing.StartSpanFromContext(ctx, "thunder.sqlgen.query")
		span.LogFields(log.String("query", clause))
		defer span.Finish()
	}

	res, err := db.QueryExecer(ctx).Query(clause, fields...)
	if err != nil {
		return nil, err
	}
	defer res.Close()

	return db.Schema.ParseRows(selectQuery, res)
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

	rows, err := db.query(ctx, query)
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

	rows, err := db.query(ctx, query)
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

	clause, args := query.ToSQL()

	if span := opentracing.SpanFromContext(ctx); span != nil {
		span, ctx = opentracing.StartSpanFromContext(ctx, "thunder.sqlgen.InsertRow")
		span.LogFields(log.String("query", clause))
		defer span.Finish()
	}

	return db.QueryExecer(ctx).Exec(clause, args...)
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

	clause, args := query.ToSQL()

	if span := opentracing.SpanFromContext(ctx); span != nil {
		span, ctx = opentracing.StartSpanFromContext(ctx, "thunder.sqlgen.UpsertRow")
		span.LogFields(log.String("query", clause))
		defer span.Finish()
	}

	return db.QueryExecer(ctx).Exec(clause, args...)
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

	clause, args := query.ToSQL()

	if span := opentracing.SpanFromContext(ctx); span != nil {
		span, ctx = opentracing.StartSpanFromContext(ctx, "thunder.sqlgen.UpdateRow")
		span.LogFields(log.String("query", clause))
		defer span.Finish()
	}

	_, err = db.QueryExecer(ctx).Exec(clause, args...)
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

	clause, args := query.ToSQL()

	if span := opentracing.SpanFromContext(ctx); span != nil {
		span, ctx = opentracing.StartSpanFromContext(ctx, "thunder.sqlgen.DeleteRow")
		span.LogFields(log.String("query", clause))
		defer span.Finish()
	}

	_, err = db.QueryExecer(ctx).Exec(clause, args...)
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
}

func (db *DB) QueryExecer(ctx context.Context) QueryExecer {
	maybeTx := ctx.Value(txKey{db: db})
	if maybeTx != nil {
		return maybeTx.(*sql.Tx)
	}
	return db.Conn
}
