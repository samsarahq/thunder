package sqlgen

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/samsarahq/thunder/batch"
	"github.com/samsarahq/thunder/internal/proto"
	"github.com/samsarahq/thunder/internal/testfixtures"
	"github.com/stretchr/testify/assert"
)

func setup() (*testfixtures.TestDatabase, *DB, error) {
	testDb, err := testfixtures.NewTestDatabase()
	if err != nil {
		return nil, nil, err
	}

	if _, err = testDb.Exec(`
		CREATE TABLE users (
			id            BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			name          VARCHAR(255),
			uuid          VARCHAR(255),
			mood          VARCHAR(255),
			proto         BLOB,
			implicit_null VARCHAR(255)
		)
	`); err != nil {
		return nil, nil, err
	}
	schema := NewSchema()
	schema.MustRegisterType("users", AutoIncrement, User{})

	return testDb, NewDB(testDb.DB, schema), nil
}

type User struct {
	Id           int64 `sql:",primary"`
	Name         string
	Uuid         testfixtures.CustomType
	Mood         *testfixtures.CustomType
	Proto        proto.ExampleEvent `sql:",binary"`
	ImplicitNull string             `sql:",implicitnull"`
}

type Complex struct {
	Id           int64 `sql:",primary"`
	Name         string
	Text         []byte            `sql:",string"`
	Blob         []byte            `sql:",binary"`
	Mappings     map[string]string `sql:",json"`
	ImplicitNull string            `sql:",implicitnull"`
}

func TestTagOverrides(t *testing.T) {
	schema := NewSchema()
	err := schema.RegisterType("complex", AutoIncrement, Complex{})
	assert.NoError(t, err)
}

func TestContextDeadlineEnforced(t *testing.T) {
	tdb, db, err := setup()
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err = db.QueryExecer(ctx).ExecContext(ctx, "DO SLEEP(1)"); err == nil || err != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got: %s", err)
	}
}

func TestIntegrationBasic(t *testing.T) {
	tdb, db, err := setup()
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.Close()

	mood := testfixtures.CustomType{'f', 'o', 'o', 'o', 'o', 'o', 'o'}

	if _, err := db.InsertRow(context.Background(), &User{Name: "Bob", Uuid: testfixtures.CustomType{'1', '1', '2', '3', '8', '4', '9', '1', '2', '9', '3'}, Mood: &mood}); err != nil {
		t.Error(err)
	}

	var users []*User
	if err := db.Query(context.Background(), &users, nil, nil); err != nil {
		t.Error(err)
	}

	assert.Equal(t, []*User{
		{
			Id:   1,
			Name: "Bob",
			Uuid: testfixtures.CustomType{'1', '1', '2', '3', '8', '4', '9', '1', '2', '9', '3'},
			Mood: &mood,
		},
	}, users)
}

// TestContextCancelBeforeRowsScan demonstrates we don't
// always get context.Canceled back from sql library. This
// affects our error handling and we need to be aware of it.
func TestContextCancelBeforeRowsScan(t *testing.T) {
	testDb, err := testfixtures.NewTestDatabase()
	if err != nil {
		t.Fatal(err)
	}
	defer testDb.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rows, err := testDb.QueryContext(ctx, `select "foo"`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	// When we cancel the context after rows.Next() returns true,
	// database/sql.(*Rows).initContextClose monitors the context
	// and closes rows asynchronously, and subsequent rows.Scan()
	// returns errors.New("sql: Rows are closed") instead of
	// context.Canceled.
	for rows.Next() {
		cancel()
		time.Sleep(1000 * time.Millisecond)

		var foo string
		err := rows.Scan(&foo)

		// err is not context.Canceled.
		if err == nil || err.Error() != "sql: Rows are closed" {
			t.Fatalf("expecting 'sql: Rows are closed' from rows.Scan(), got %v", err)
		}
	}
	if err := rows.Err(); err != context.Canceled {
		t.Fatalf("expecting context.Canceled from rows.Err(), got %v", err)
	}
}

// TestBatchFilter shows sqlgen batching matcher does not match if
// filter type does not exactly match column type.
func TestBatchFilter(t *testing.T) {
	tdb, db, err := setup()
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.Close()

	if _, err := db.InsertRow(context.Background(), &User{
		Id:   1,
		Name: "Bob",
		Uuid: testfixtures.CustomType{'1', '1', '2', '3', '8', '4', '9', '1', '2', '9', '3'},
		Mood: &testfixtures.CustomType{'f', 'o', 'o', 'o', 'o', 'o', 'o'},
	}); err != nil {
		t.Fatal(err)
	}

	ctx := batch.WithBatching(context.Background())

	var user *User

	// Only int64 works because id field is int64.
	if err := db.QueryRow(ctx, &user, Filter{"id": int64(1)}, nil); err != nil {
		t.Fatal(err)
	}

	// Other int variants don't work.
	if err := db.QueryRow(ctx, &user, Filter{"id": int32(1)}, nil); err != sql.ErrNoRows {
		t.Fatalf("expecting sql.ErrNoRows, got: %v", err)
	}
	if err := db.QueryRow(ctx, &user, Filter{"id": int16(1)}, nil); err != sql.ErrNoRows {
		t.Fatalf("expecting sql.ErrNoRows, got: %v", err)
	}
	if err := db.QueryRow(ctx, &user, Filter{"id": int8(1)}, nil); err != sql.ErrNoRows {
		t.Fatalf("expecting sql.ErrNoRows, got: %v", err)
	}
	if err := db.QueryRow(ctx, &user, Filter{"id": int(1)}, nil); err != sql.ErrNoRows {
		t.Fatalf("expecting sql.ErrNoRows, got: %v", err)
	}

	// Unsigned int does not work.
	if err := db.QueryRow(ctx, &user, Filter{"id": uint(1)}, nil); err != sql.ErrNoRows {
		t.Fatalf("expecting sql.ErrNoRows, got: %v", err)
	}

	// String does not work either.
	if err := db.QueryRow(ctx, &user, Filter{"id": "1"}, nil); err != sql.ErrNoRows {
		t.Fatalf("expecting sql.ErrNoRows, got: %v", err)
	}
}

func Benchmark(b *testing.B) {
	tdb, db, err := setup()
	if err != nil {
		b.Fatal(err)
	}
	defer tdb.Close()

	ctx := context.Background()

	mood := testfixtures.CustomType{'f', 'o', 'o', 'o', 'o', 'o', 'o'}
	user := &User{
		Id:           1,
		Name:         "Bob",
		Uuid:         testfixtures.CustomType{'1', '1', '2', '3', '8', '4', '9', '1', '2', '9', '3'},
		Mood:         &mood,
		ImplicitNull: "test",
	}

	if _, err := db.InsertRow(ctx, user); err != nil {
		b.Fatal(err)
	}

	benchmarks := []struct {
		name string
		fn   func() error
	}{
		{"Read", func() error {
			user := &User{}
			return db.QueryRow(ctx, &user, nil, nil)
		}},
		{"Read_Where", func() error {
			user := &User{}
			return db.QueryRow(ctx, &user, Filter{"name": "Bob"}, nil)
		}},
		{"Create", func() error {
			_, err := db.InsertRow(ctx, user)
			return err
		}},
		{"Update", func() error { return db.UpdateRow(ctx, user) }},
		{"Delete", func() error { return db.DeleteRow(ctx, user) }},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if err := bm.fn(); err != nil {
					b.Error(err)
				}
			}
		})
	}
}
