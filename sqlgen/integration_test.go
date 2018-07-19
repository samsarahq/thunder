package sqlgen

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/kylelemons/godebug/pretty"
)

const base = "root:@tcp(localhost:3307)/"

type TestDatabase struct {
	ControlDB *sql.DB
	DBName    string
	*sql.DB
}

func NewTestDatabase() (*TestDatabase, error) {
	controlDb, err := sql.Open("mysql", base)
	if err != nil {
		return nil, err
	}

	name := fmt.Sprintf("thunder_test_%d", rand.Intn(1<<30))
	_, err = controlDb.Exec(fmt.Sprintf("CREATE DATABASE %s", name))
	if err != nil {
		controlDb.Close()
		return nil, err
	}

	db, err := sql.Open("mysql", base+name)
	if err != nil {
		controlDb.Close()
		return nil, err
	}

	return &TestDatabase{
		DB:        db,
		DBName:    name,
		ControlDB: controlDb,
	}, nil
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *TestDatabase) Close() error {
	first := t.DB.Close()
	_, second := t.ControlDB.Exec(fmt.Sprintf("DROP DATABASE %s", t.DBName))
	third := t.ControlDB.Close()
	return firstError(first, second, third)
}

func TestContextDeadlineEnforced(t *testing.T) {
	testDb, err := NewTestDatabase()
	if err != nil {
		t.Fatal(err)
	}
	defer testDb.Close()

	schema := NewSchema()
	db := NewDB(testDb.DB, schema)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err = db.QueryExecer(ctx).ExecContext(ctx, "DO SLEEP(1)"); err == nil || err != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got: %s", err)
	}
}

type mockType []byte

func (u *mockType) Value() (driver.Value, error) {
	return []byte(*u), nil
}

func (u *mockType) Scan(value interface{}) error {
	switch value := value.(type) {
	case nil:
		u = nil
	case []byte:
		*u = make(mockType, len(value))
		copy(*u, value)
	case string:
		*u = mockType(value)
	default:
		return fmt.Errorf("cannot convert %v (of type %T) to %T", value, value, u)
	}
	return nil
}

func TestIntegrationBasic(t *testing.T) {
	testDb, err := NewTestDatabase()
	if err != nil {
		t.Fatal(err)
	}
	defer testDb.Close()

	_, err = testDb.Exec(`
		CREATE TABLE users (
			id   BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			name VARCHAR(255),
			uuid VARCHAR(255),
			mood VARCHAR(255)
		)
	`)
	if err != nil {
		t.Fatal(err)
	}

	type User struct {
		Id   int64 `sql:",primary"`
		Name string
		Uuid mockType
		Mood *mockType
	}
	schema := NewSchema()
	schema.MustRegisterType("users", AutoIncrement, User{})
	mood := mockType("foooooo")

	db := NewDB(testDb.DB, schema)
	if _, err := db.InsertRow(context.Background(), &User{Name: "Bob", Uuid: mockType("11238491293"), Mood: &mood}); err != nil {
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
			Uuid: mockType("11238491293"),
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
	testDb, err := NewTestDatabase()
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
