package sqlgen

import (
	"context"
	"testing"
	"time"

	"github.com/kylelemons/godebug/pretty"
	"github.com/samsarahq/thunder/internal/testfixtures"
)

func setup() (*testfixtures.TestDatabase, *DB, error) {
	testDb, err := testfixtures.NewTestDatabase()
	if err != nil {
		return nil, nil, err
	}

	if _, err = testDb.Exec(`
		CREATE TABLE users (
			id   BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			name VARCHAR(255),
			uuid VARCHAR(255),
			mood VARCHAR(255)
		)
	`); err != nil {
		return nil, nil, err
	}
	schema := NewSchema()
	schema.MustRegisterType("users", AutoIncrement, User{})

	return testDb, NewDB(testDb.DB, schema), nil
}

type User struct {
	Id   int64 `sql:",primary"`
	Name string
	Uuid testfixtures.CustomType
	Mood *testfixtures.CustomType
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

	if diff := pretty.Compare(users, []*User{
		{
			Id:   1,
			Name: "Bob",
			Uuid: testfixtures.CustomType{'1', '1', '2', '3', '8', '4', '9', '1', '2', '9', '3'},
			Mood: &mood,
		},
	}); diff != "" {
		t.Errorf("diff: %s", diff)
	}
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
