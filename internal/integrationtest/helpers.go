package integrationtest

import (
	"context"
	"database/sql"
	"testing"

	"github.com/northvolt/thunder/internal/testfixtures"
	"github.com/northvolt/thunder/livesql"
	"github.com/northvolt/thunder/sqlgen"
	"github.com/stretchr/testify/require"
)

// DB is an interface that satisfies both sqlgen.DB and LiveDB
type DB interface {
	Query(ctx context.Context, result interface{}, filter sqlgen.Filter, options *sqlgen.SelectOptions) error
	QueryRow(ctx context.Context, result interface{}, filter sqlgen.Filter, options *sqlgen.SelectOptions) error
	InsertRow(ctx context.Context, row interface{}) (sql.Result, error)
	UpsertRow(ctx context.Context, row interface{}) (sql.Result, error)
	UpdateRow(ctx context.Context, row interface{}) error
	DeleteRow(ctx context.Context, row interface{}) error
}

// DBGenerator is a struct that contains the ability to build a database.
type DBGenerator struct {
	Name      string
	Generator func(t *testing.T) (db DB, sqlDB *sql.DB, close func())
}

// NewDBGenerators is a function that will return a list of database
// "generators" which own the lifecycle of a database (both live and sqlgen)
func NewDBGenerators(dbSetup func(testDB *testfixtures.TestDatabase, schema *sqlgen.Schema) error) []DBGenerator {
	return []DBGenerator{
		{
			Name: "sqlgen",
			Generator: func(t *testing.T) (db DB, sqlDB *sql.DB, close func()) {
				testDb, err := testfixtures.NewTestDatabase()
				require.NoError(t, err)
				schema := sqlgen.NewSchema()

				require.NoError(t, dbSetup(testDb, schema))

				return sqlgen.NewDB(testDb.DB, schema), testDb.DB, func() { require.NoError(t, testDb.Close()) }
			},
		},
		{
			Name: "livedb",
			Generator: func(t *testing.T) (db DB, sqlDB *sql.DB, close func()) {
				testDb, err := testfixtures.NewTestDatabase()
				require.NoError(t, err)
				schema := sqlgen.NewSchema()

				require.NoError(t, dbSetup(testDb, schema))

				return livesql.NewLiveDB(sqlgen.NewDB(testDb.DB, schema)), testDb.DB, func() { require.NoError(t, testDb.Close()) }
			},
		},
	}
}
