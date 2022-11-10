package sqlgen

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/samsarahq/thunder/batch"
	"github.com/samsarahq/thunder/internal/proto"
	"github.com/samsarahq/thunder/internal/testfixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			simple_proto  BLOB,
			implicit_null VARCHAR(255)
		)
	`); err != nil {
		return nil, nil, err
	}
	if _, err = testDb.Exec(`
		CREATE TABLE just_ids (
			id            BIGINT NOT NULL PRIMARY KEY
		)
	`); err != nil {
		return nil, nil, err
	}

	if _, err = testDb.Exec(`
		CREATE TABLE owls (
			species VARCHAR(100) NOT NULL,
			common_name VARCHAR(100) NOT NULL,
			genus VARCHAR(100) NOT NULL,
			family VARCHAR(100) NOT NULL,
			PRIMARY KEY (species),
			KEY family_and_genus_index (family, genus)
		)
	`); err != nil {
		return nil, nil, err
	}

	schema := NewSchema()
	schema.MustRegisterType("users", AutoIncrement, User{})
	schema.MustRegisterType("just_ids", UniqueId, JustId{})
	schema.MustRegisterType("owls", UniqueId, Owl{})

	return testDb, NewDB(testDb.DB, schema), nil
}

type User struct {
	Id           int64 `sql:",primary"`
	Name         string
	Uuid         testfixtures.CustomType
	Mood         *testfixtures.CustomType
	Proto        proto.ExampleEvent       `sql:",binary"`
	SimpleProto  proto.SimpleExampleEvent `sql:",binary"`
	ImplicitNull string                   `sql:",implicitnull"`
}

type JustId struct {
	Id int64 `sql:",primary"`
}

type Complex struct {
	Id           int64 `sql:",primary"`
	Name         string
	Text         []byte            `sql:",string"`
	Blob         []byte            `sql:",binary"`
	Mappings     map[string]string `sql:",json"`
	ImplicitNull string            `sql:",implicitnull"`
}

type Owl struct {
	Species    string `sql:",primary"`
	CommonName string
	Genus      string
	Family     string
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

	numBobs, err := db.Count(context.Background(), &User{}, Filter{"name": "Bob"})
	assert.NoError(t, err)
	assert.Equal(t, int64(1), numBobs)
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

func TestSelectForUpdate(t *testing.T) {
	initialDbState := []*Owl{
		{Species: "Tyto alba", CommonName: "Barn Owl", Genus: "Tyto", Family: "Tytonidae"},
		{Species: "Bubo bubo", CommonName: "Eurasian Eagle-Owl", Genus: "Bubo", Family: "Strigidae"},
		{Species: "Bubo virginianus", CommonName: "Great Horned Owl", Genus: "Bubo", Family: "Strigidae"},
		{Species: "Megascops kennicottii", CommonName: "Western Screech-Owl", Genus: "Megascops", Family: "Strigidae"},
		{Species: "Psiloscops flammeolus", CommonName: "Flammulated Owl", Genus: "Psiloscops", Family: "Strigidae"},
		{Species: "Strix occidentalis lucida", CommonName: "Mexican Spotted Owl", Genus: "Strix", Family: "Strigidae"},
	}

	testCases := []struct {
		name                    string
		description             string
		mainThreadFilter        Filter
		mainThreadOptions       *SelectOptions
		goroutineFilter         Filter
		goroutineOptions        *SelectOptions
		mainThreadUpdateFunc    func(ctx context.Context, db *DB, waiter *sync.WaitGroup) error
		goroutineExpectedResult []*Owl
	}{
		{
			name:              "main thread blocks goroutine for same query",
			description:       "The main thread and the goroutine both SELECT...FOR UPDATE the same rows. After the main thread performs an UPDATE and a DELETE, the goroutine should be unblocked to SELECT the updated data",
			mainThreadFilter:  Filter{"family": "Strigidae", "genus": "Bubo"},
			mainThreadOptions: &SelectOptions{ForUpdate: true},
			goroutineFilter:   Filter{"family": "Strigidae", "genus": "Bubo"},
			goroutineOptions:  &SelectOptions{ForUpdate: true},
			mainThreadUpdateFunc: func(ctx context.Context, db *DB, waiter *sync.WaitGroup) error {
				// Delete one of the rows and update the other row.
				if err := db.DeleteRow(ctx, &Owl{Species: "Bubo virginianus"}); err != nil {
					return err
				}
				if err := db.UpdateRow(ctx, &Owl{Species: "Bubo bubo", CommonName: "TESTING TESTING 1 2 3", Genus: "Bubo", Family: "Strigidae"}); err != nil {
					return err
				}
				return nil
			},
			goroutineExpectedResult: []*Owl{
				{Species: "Bubo bubo", CommonName: "TESTING TESTING 1 2 3", Genus: "Bubo", Family: "Strigidae"},
			},
		},
		{
			name:              "goroutine does dirty read",
			description:       "The main thread does a SELECT...FOR UPDATE, but the goroutine does a basic SELECT to get a 'dirty read'. Even though the main thread updates the data, the goroutine has read the original data",
			mainThreadFilter:  Filter{"family": "Strigidae", "genus": "Bubo"},
			mainThreadOptions: &SelectOptions{ForUpdate: true},
			goroutineFilter:   Filter{"family": "Strigidae", "genus": "Bubo"},
			goroutineOptions:  &SelectOptions{OrderBy: "species"},
			mainThreadUpdateFunc: func(ctx context.Context, db *DB, waiter *sync.WaitGroup) error {
				// Wait until the goroutine has performed its SELECT.
				// Then, DELETE one of the rows and UPDATE the other row.
				waiter.Wait()

				if err := db.DeleteRow(ctx, &Owl{Species: "Bubo virginianus"}); err != nil {
					return err
				}
				if err := db.UpdateRow(ctx, &Owl{Species: "Bubo bubo", CommonName: "TESTING TESTING 1 2 3", Genus: "Bubo", Family: "Strigidae"}); err != nil {
					return err
				}
				return nil
			},
			goroutineExpectedResult: []*Owl{
				{Species: "Bubo bubo", CommonName: "Eurasian Eagle-Owl", Genus: "Bubo", Family: "Strigidae"},
				{Species: "Bubo virginianus", CommonName: "Great Horned Owl", Genus: "Bubo", Family: "Strigidae"},
			},
		},
		{
			name:              "main thread blocks with broad query",
			description:       "Main thread performs a broad SELECT ... FOR UPDATE. The goroutine does a narrow SELECT ... FOR UPDATE and should be blocked by the main thread's query. The main thread INSERTs a new row, and the goroutine should pick it up.",
			mainThreadFilter:  Filter{"family": "Strigidae"},
			mainThreadOptions: &SelectOptions{ForUpdate: true},
			goroutineFilter:   Filter{"family": "Strigidae", "genus": "Megascops"},
			goroutineOptions:  &SelectOptions{ForUpdate: true, OrderBy: "species"},
			mainThreadUpdateFunc: func(ctx context.Context, db *DB, waiter *sync.WaitGroup) error {
				// INSERT A new row that the goroutine should get in its SELECT query.
				if _, err := db.InsertRow(ctx, &Owl{Species: "Megascops asio", CommonName: "Eastern Screech Owl", Genus: "Megascops", Family: "Strigidae"}); err != nil {
					return err
				}
				return nil
			},
			goroutineExpectedResult: []*Owl{
				{Species: "Megascops asio", CommonName: "Eastern Screech Owl", Genus: "Megascops", Family: "Strigidae"},
				{Species: "Megascops kennicottii", CommonName: "Western Screech-Owl", Genus: "Megascops", Family: "Strigidae"},
			},
		},
		{
			name:              "non-blocking queries on different parts of the index",
			description:       "The main thread and the goroutine both perform a SELECT ... FOR UPDATE, but their queries shouldn't block each other because they are selecting different parts of the index",
			mainThreadFilter:  Filter{"family": "Strigidae", "genus": "Megascops"},
			mainThreadOptions: &SelectOptions{ForUpdate: true},
			goroutineFilter:   Filter{"family": "Strigidae", "genus": "Psiloscops"},
			goroutineOptions:  &SelectOptions{ForUpdate: true},
			mainThreadUpdateFunc: func(ctx context.Context, db *DB, waiter *sync.WaitGroup) error {
				// Wait until the goroutine has performed its SELECT.
				// Then, INSERT a new row.
				waiter.Wait()

				if _, err := db.InsertRow(ctx, &Owl{Species: "Megascops asio", CommonName: "Eastern Screech Owl", Genus: "Megascops", Family: "Strigidae"}); err != nil {
					return err
				}
				return nil
			},
			goroutineExpectedResult: []*Owl{
				{Species: "Psiloscops flammeolus", CommonName: "Flammulated Owl", Genus: "Psiloscops", Family: "Strigidae"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create the test database and insert some initial rows into the owls table.
			tdb, db, err := setup()
			require.NoError(t, err)
			defer tdb.Close()

			err = db.InsertRows(context.Background(), initialDbState, 100)
			require.NoError(t, err)

			// Begin a transaction and SELECT ... FOR UPDATE some rows.
			txCtx, tx, err := db.WithTx(context.Background())
			require.NoError(t, err)
			defer tx.Rollback()

			var result []*Owl
			err = db.Query(txCtx, &result, tc.mainThreadFilter, tc.mainThreadOptions)
			require.NoError(t, err)

			// In a separate goroutine, begin a new transaction and perform a SELECT statement.
			// Depending on the test case, this goroutine may be blocked by the main thread's SELECT...FOR UPDATE
			// statement. In other cases, the SELECT statement isn't blocking, and the goroutine should
			// finish immediately.
			//
			// The goroutine's results will be returned to the main thread via this returnChan channel.
			returnChan := make(chan []*Owl)
			// When the goroutine's SELECT is complete, it'll signal that it's done using this WaitGroup.
			// This will be used in cases where we want the main thread to wait, in order to verify that the
			// goroutine was not blocked.
			goroutineWaiter := &sync.WaitGroup{}
			goroutineWaiter.Add(1)
			go func(db *DB) {
				// Create the new transaction.
				goroutineTxCtx, goroutineTx, err := db.WithTx(context.Background())
				require.NoError(t, err)
				defer tx.Rollback()

				// Perform a SELECT statement.
				var result []*Owl
				err = db.Query(goroutineTxCtx, &result, tc.goroutineFilter, tc.goroutineOptions)
				require.NoError(t, err)

				// Signal that the goroutine is done, commit, and return.
				goroutineWaiter.Done()
				err = goroutineTx.Commit()
				require.NoError(t, err)
				returnChan <- result
			}(db)

			// Perform some actions and commit the transaction.
			err = tc.mainThreadUpdateFunc(txCtx, db, goroutineWaiter)
			require.NoError(t, err)

			err = tx.Commit()
			require.NoError(t, err)

			// Wait for the goroutine to return the result through the channel.
			goroutineResult := <-returnChan

			// Verify that the inner goroutine's SELECT statement returned the expected result.
			assert.Equal(t, tc.goroutineExpectedResult, goroutineResult)
		})
	}
}

func TestForceIndex(t *testing.T) {
	initialDbState := []*Owl{
		{Species: "Tyto alba", CommonName: "Barn Owl", Genus: "Tyto", Family: "Tytonidae"},
		{Species: "Bubo bubo", CommonName: "Eurasian Eagle-Owl", Genus: "Bubo", Family: "Strigidae"},
		{Species: "Bubo virginianus", CommonName: "Great Horned Owl", Genus: "Bubo", Family: "Strigidae"},
		{Species: "Megascops kennicottii", CommonName: "Western Screech-Owl", Genus: "Megascops", Family: "Strigidae"},
		{Species: "Psiloscops flammeolus", CommonName: "Flammulated Owl", Genus: "Psiloscops", Family: "Strigidae"},
		{Species: "Strix occidentalis lucida", CommonName: "Mexican Spotted Owl", Genus: "Strix", Family: "Strigidae"},
	}

	testCases := []struct {
		description         string
		queryFilter         Filter
		queryOptions        *SelectOptions
		expectedErrorString string
	}{
		{
			description:  "Specify an index to use",
			queryFilter:  Filter{"family": "Strigidae", "genus": "Bubo"},
			queryOptions: &SelectOptions{ForceIndex: []string{"family_and_genus_index"}},
		},
		{
			description:  "Specify multiple index to consider",
			queryFilter:  Filter{"family": "Strigidae", "genus": "Bubo"},
			queryOptions: &SelectOptions{ForceIndex: []string{"family_and_genus_index", "PRIMARY"}},
		},
		{
			description:         "Specify an index that doesn't exist",
			queryFilter:         Filter{"family": "Strigidae", "genus": "Bubo"},
			queryOptions:        &SelectOptions{ForceIndex: []string{"an_index_that_doesnt_exist"}},
			expectedErrorString: "Error 1176: Key 'an_index_that_doesnt_exist' doesn't exist in table 'owls'",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Create the test database and insert some initial rows into the owls table.
			tdb, db, err := setup()
			require.NoError(t, err)
			defer tdb.Close()

			err = db.InsertRows(ctx, initialDbState, 100)
			require.NoError(t, err)

			// Execute the specified query
			var result []*Owl
			err = db.Query(ctx, &result, tc.queryFilter, tc.queryOptions)

			// Verify the result
			if tc.expectedErrorString == "" {
				assert.NoError(t, err)
				assert.Len(t, result, 2)
			} else {
				assert.Contains(t, err.Error(), tc.expectedErrorString)
			}
		})
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

func BenchmarkSql(b *testing.B) {
	tdb, db, err := setup()
	if err != nil {
		b.Fatal(err)
	}
	defer tdb.Close()

	ctx := context.Background()

	mood := testfixtures.CustomType{'f', 'o', 'o', 'o', 'o', 'o', 'o'}
	user := &User{
		Id:   1,
		Name: "Bob",
		Uuid: testfixtures.CustomType{'1', '1', '2', '3', '8', '4', '9', '1', '2', '9', '3'},
		Mood: &mood,
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
			row := db.Conn.QueryRowContext(ctx, `SELECT id, name, uuid, mood from users`)
			return row.Scan(&user.Id, &user.Name, &user.Uuid, &user.Mood)
		}},
		{"Read_Where", func() error {
			user := &User{}
			row := db.Conn.QueryRowContext(ctx, `SELECT id, name, uuid, mood from users where users.name = ?`, "Bob")
			return row.Scan(&user.Id, &user.Name, &user.Uuid, &user.Mood)
		}},
		{"Create", func() error {
			_, err := db.Conn.ExecContext(ctx, `INSERT INTO users (name, uuid, mood) VALUES (?, ?, ?)`, user.Name, user.Uuid, user.Mood)
			return err
		}},
		{"Update", func() error {
			_, err := db.Conn.ExecContext(ctx, `UPDATE users SET name = ?, uuid = ?, mood = ? WHERE users.id = ?`, user.Name, user.Uuid, user.Mood, user.Id)
			return err
		}},
		{"Delete", func() error {
			_, err := db.Conn.ExecContext(ctx, `DELETE FROM users WHERE users.id = ?`, user.Id)
			return err
		}},
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
