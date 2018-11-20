package integrationtest

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/samsarahq/thunder/batch"
	"github.com/samsarahq/thunder/internal/testfixtures"
	"github.com/samsarahq/thunder/sqlgen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ImplicitNull struct {
	Id        int64     `sql:",primary"`
	NullStr   string    `sql:",implicitnull"`
	NullInt   int64     `sql:",implicitnull"`
	NullFloat float64   `sql:",implicitnull"`
	NullBool  bool      `sql:",implicitnull"`
	NullByte  []byte    `sql:",implicitnull"`
	NullTime  time.Time `sql:",implicitnull"`
}

func TestImplicitNullIsNullInDB(t *testing.T) {
	tests := []struct {
		name              string
		giveStruct        *ImplicitNull
		wantNullFields    []string
		wantNonNullFields []string
	}{
		{
			name:           "all null",
			giveStruct:     &ImplicitNull{},
			wantNullFields: []string{"null_str", "null_int", "null_float", "null_bool", "null_byte", "null_time"},
		},
		{
			name: "no null",
			giveStruct: &ImplicitNull{
				NullStr:   "hello",
				NullInt:   123,
				NullFloat: 1.23,
				NullBool:  true,
				NullByte:  []byte("hello there"),
				NullTime:  time.Date(2000, 10, 3, 0, 0, 0, 0, time.UTC),
			},
			wantNonNullFields: []string{"null_str", "null_int", "null_float", "null_bool", "null_byte", "null_time"},
		},
	}

	for _, dbGen := range getImplicitDBGenerators() {
		for _, testctx := range getTestContexts() {
			for _, tt := range tests {
				t.Run(fmt.Sprintf("%s-%s-%s", dbGen.Name, tt.name, testctx.name), func(t *testing.T) {
					db, sqlDb, closer := dbGen.Generator(t)
					defer closer()
					ctx := testctx.ctx

					if _, err := db.InsertRow(ctx, tt.giveStruct); err != nil {
						t.Error(err)
					}

					for _, field := range tt.wantNullFields {
						assertFieldIsNull(t, sqlDb, field)
					}

					for _, field := range tt.wantNonNullFields {
						assertFieldIsNotNull(t, sqlDb, field)
					}

					var res *ImplicitNull
					require.NoError(t, db.QueryRow(ctx, &res, nil, nil))
					tt.giveStruct.Id = 1
					assert.Equal(t, tt.giveStruct, res)
				})
			}
		}
	}
}

func TestInvalidImplicitNullField(t *testing.T) {
	type InvalidImplicitNull struct {
		Id      int64   `sql:",primary"`
		NullStr *string `sql:",implicitnull"` // Should not allow pointers
	}

	schema := sqlgen.NewSchema()
	err := schema.RegisterType("invalidimplicitnull", sqlgen.AutoIncrement, InvalidImplicitNull{})

	require.Error(t, err)
	require.Contains(t, err.Error(), "column null_str cannot use `implicitnull` with pointer type")
}

func TestImplicitNullFiltersWorkForNonNull(t *testing.T) {
	for _, dbGen := range getImplicitDBGenerators() {
		for _, tt := range getTestContexts() {
			t.Run(fmt.Sprintf("%s-%s", dbGen.Name, tt.name), func(t *testing.T) {
				db, _, closer := dbGen.Generator(t)
				defer closer()
				ctx := tt.ctx

				timeField := time.Date(2000, 10, 3, 0, 0, 0, 0, time.UTC)
				newRow := &ImplicitNull{
					NullStr:   "hello",
					NullInt:   123,
					NullFloat: 1.23,
					NullBool:  true,
					NullByte:  []byte("hello there"),
					NullTime:  timeField,
				}
				if _, err := db.InsertRow(ctx, newRow); err != nil {
					t.Error(err)
				}
				var imp *ImplicitNull

				require.NoError(t, db.QueryRow(ctx, &imp, sqlgen.Filter{"id": int64(1)}, nil))
				require.NoError(t, db.QueryRow(ctx, &imp, sqlgen.Filter{"null_str": "hello"}, nil))
				require.NoError(t, db.QueryRow(ctx, &imp, sqlgen.Filter{"null_int": int64(123)}, nil))
				require.NoError(t, db.QueryRow(ctx, &imp, sqlgen.Filter{"null_float": 1.23}, nil))
				require.NoError(t, db.QueryRow(ctx, &imp, sqlgen.Filter{"null_bool": true}, nil))
				require.NoError(t, db.QueryRow(ctx, &imp, sqlgen.Filter{"null_byte": []byte("hello there")}, nil))
				require.NoError(t, db.QueryRow(ctx, &imp, sqlgen.Filter{"null_time": timeField}, nil))

				newRow.Id = 1
				var rows []*ImplicitNull
				require.NoError(t, db.Query(context.Background(), &rows, nil, nil))
				assert.Equal(t, []*ImplicitNull{
					newRow,
				}, rows)
			})
		}
	}
}

func TestImplicitNullFillsWithZeroValue(t *testing.T) {
	for _, dbGen := range getImplicitDBGenerators() {
		for _, tt := range getTestContexts() {
			t.Run(fmt.Sprintf("%s-%s", dbGen.Name, tt.name), func(t *testing.T) {
				db, _, closer := dbGen.Generator(t)
				defer closer()
				ctx := tt.ctx

				newRow := &ImplicitNull{}
				if _, err := db.InsertRow(ctx, newRow); err != nil {
					t.Error(err)
				}

				var imp *ImplicitNull
				require.NoError(t, db.QueryRow(ctx, &imp, sqlgen.Filter{"id": int64(1)}, nil))

				expectedRow := &ImplicitNull{Id: 1} // The rest is zero value
				assert.Equal(t, expectedRow, imp)
			})
		}
	}
}

func TestImplicitNullUpdate(t *testing.T) {
	tests := []struct {
		name              string
		giveStruct        *ImplicitNull
		giveUpdate        *ImplicitNull
		wantResult        *ImplicitNull
		wantNullFields    []string
		wantNonNullFields []string
	}{
		{
			name:       "all null updated",
			giveStruct: &ImplicitNull{},
			giveUpdate: &ImplicitNull{
				NullStr:   "hello",
				NullInt:   123,
				NullFloat: 1.23,
				NullBool:  true,
				NullByte:  []byte("hello there"),
				NullTime:  time.Date(2000, 10, 3, 0, 0, 0, 0, time.UTC),
			},
			wantResult: &ImplicitNull{
				NullStr:   "hello",
				NullInt:   123,
				NullFloat: 1.23,
				NullBool:  true,
				NullByte:  []byte("hello there"),
				NullTime:  time.Date(2000, 10, 3, 0, 0, 0, 0, time.UTC),
			},
			wantNonNullFields: []string{"null_str", "null_int", "null_float", "null_bool", "null_byte", "null_time"},
		},
		{
			name: "all updated to null",
			giveStruct: &ImplicitNull{
				NullStr:   "hello",
				NullInt:   123,
				NullFloat: 1.23,
				NullBool:  true,
				NullByte:  []byte("hello there"),
				NullTime:  time.Date(2000, 10, 3, 0, 0, 0, 0, time.UTC),
			},
			giveUpdate:     &ImplicitNull{},
			wantResult:     &ImplicitNull{},
			wantNullFields: []string{"null_str", "null_int", "null_float", "null_bool", "null_byte", "null_time"},
		},
		{
			name: "partial update",
			giveStruct: &ImplicitNull{
				NullStr:   "hello",
				NullInt:   123,
				NullFloat: 1.23,
			},
			giveUpdate: &ImplicitNull{
				NullFloat: 1.23,
				NullBool:  true,
				NullByte:  []byte("hello there"),
				NullTime:  time.Date(2000, 10, 3, 0, 0, 0, 0, time.UTC),
			},
			wantResult: &ImplicitNull{
				NullFloat: 1.23,
				NullBool:  true,
				NullByte:  []byte("hello there"),
				NullTime:  time.Date(2000, 10, 3, 0, 0, 0, 0, time.UTC),
			},
			wantNullFields:    []string{"null_str", "null_int"},
			wantNonNullFields: []string{"null_float", "null_bool", "null_byte", "null_time"},
		},
	}

	for _, dbGen := range getImplicitDBGenerators() {
		for _, testctx := range getTestContexts() {
			for _, tt := range tests {
				t.Run(fmt.Sprintf("%s-%s-%s", dbGen.Name, tt.name, testctx.name), func(t *testing.T) {
					db, sqlDB, closer := dbGen.Generator(t)
					defer closer()

					ctx := testctx.ctx

					if _, err := db.InsertRow(ctx, tt.giveStruct); err != nil {
						t.Error(err)
					}

					tt.giveUpdate.Id = 1
					if err := db.UpdateRow(ctx, tt.giveUpdate); err != nil {
						t.Error(err)
					}

					for _, field := range tt.wantNullFields {
						assertFieldIsNull(t, sqlDB, field)
					}

					for _, field := range tt.wantNonNullFields {
						assertFieldIsNotNull(t, sqlDB, field)
					}

					var res *ImplicitNull
					require.NoError(t, db.QueryRow(ctx, &res, nil, nil))
					tt.wantResult.Id = 1
					assert.Equal(t, tt.wantResult, res)
				})
			}
		}
	}
}

func TestImplicitNullDelete(t *testing.T) {
	tests := []struct {
		name       string
		giveStruct *ImplicitNull
		giveDelete *ImplicitNull
	}{
		{
			name:       "all null delete (same)",
			giveStruct: &ImplicitNull{},
			giveDelete: &ImplicitNull{},
		},
		{
			name: "all nonnull delete (same)",
			giveStruct: &ImplicitNull{
				NullStr:   "hello",
				NullInt:   123,
				NullFloat: 1.23,
				NullBool:  true,
				NullByte:  []byte("hello there"),
				NullTime:  time.Date(2000, 10, 3, 0, 0, 0, 0, time.UTC),
			},
			giveDelete: &ImplicitNull{
				NullStr:   "hello",
				NullInt:   123,
				NullFloat: 1.23,
				NullBool:  true,
				NullByte:  []byte("hello there"),
				NullTime:  time.Date(2000, 10, 3, 0, 0, 0, 0, time.UTC),
			},
		},
		{
			name:       "all null deleted by full",
			giveStruct: &ImplicitNull{},
			giveDelete: &ImplicitNull{
				NullStr:   "hello",
				NullInt:   123,
				NullFloat: 1.23,
				NullBool:  true,
				NullByte:  []byte("hello there"),
				NullTime:  time.Date(2000, 10, 3, 0, 0, 0, 0, time.UTC),
			},
		},
		{
			name: "all null deleted by full",
			giveStruct: &ImplicitNull{
				NullStr:   "hello",
				NullInt:   123,
				NullFloat: 1.23,
				NullBool:  true,
				NullByte:  []byte("hello there"),
				NullTime:  time.Date(2000, 10, 3, 0, 0, 0, 0, time.UTC),
			},
			giveDelete: &ImplicitNull{},
		},
	}

	for _, dbGen := range getImplicitDBGenerators() {
		for _, testctx := range getTestContexts() {
			for _, tt := range tests {
				t.Run(fmt.Sprintf("%s-%s-%s", dbGen.Name, tt.name, testctx.name), func(t *testing.T) {
					db, _, closer := dbGen.Generator(t)
					defer closer()

					ctx := testctx.ctx

					if _, err := db.InsertRow(ctx, tt.giveStruct); err != nil {
						t.Error(err)
					}

					tt.giveDelete.Id = 1
					if err := db.DeleteRow(ctx, tt.giveDelete); err != nil {
						t.Error(err)
					}

					var res *ImplicitNull
					err := db.QueryRow(ctx, &res, nil, nil)
					require.Error(t, err)
					require.Equal(t, err, sql.ErrNoRows)
				})
			}
		}
	}
}

func getImplicitDBGenerators() []DBGenerator {
	return NewDBGenerators(
		func(testDB *testfixtures.TestDatabase, schema *sqlgen.Schema) error {
			if _, err := testDB.Exec(`
		CREATE TABLE implicitnull (
			id   BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			null_str VARCHAR(255),
			null_int BIGINT,
			null_float DOUBLE,
			null_bool TINYINT(1),
			null_byte BLOB,
			null_time DATETIME(6)
		)
	`); err != nil {
				return err
			}
			schema.MustRegisterType("implicitnull", sqlgen.AutoIncrement, ImplicitNull{})
			return nil
		},
	)
}

func getTestContexts() []struct {
	name string
	ctx  context.Context
} {
	return []struct {
		name string
		ctx  context.Context
	}{
		{"batch", batch.WithBatching(context.Background())},
		{"background", context.Background()},
	}
}

func assertFieldIsNull(t *testing.T, db *sql.DB, field string) {
	count, err := getCount(db, fmt.Sprintf("SELECT COUNT(*) FROM implicitnull WHERE %s IS NULL", field))
	require.NoError(t, err)
	assert.Equal(t, 1, count, "Expected %s to be null", field)
}

func assertFieldIsNotNull(t *testing.T, db *sql.DB, field string) {
	count, err := getCount(db, fmt.Sprintf("SELECT COUNT(*) FROM implicitnull WHERE %s IS NULL", field))
	require.NoError(t, err)
	assert.Equal(t, 0, count, "Expected %s to not be null", field)
}

func getCount(db *sql.DB, countQuery string) (int, error) {
	dbrows, err := db.Query(countQuery)
	if err != nil {
		return 0, err
	}

	var c int
	for dbrows.Next() {
		if err := dbrows.Scan(&c); err != nil {
			return 0, err
		}
	}

	return c, nil
}
