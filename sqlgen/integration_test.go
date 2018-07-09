package sqlgen

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"testing"

	"github.com/samsarahq/thunder/thunderpb"

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

func TestIntegrationBasic(t *testing.T) {
	testDb, err := NewTestDatabase()
	if err != nil {
		t.Fatal(err)
	}
	defer testDb.Close()

	_, err = testDb.Exec(`
		CREATE TABLE users (
			id    BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			name  VARCHAR(255),
			proto BLOB
		)
	`)
	if err != nil {
		t.Fatal(err)
	}

	type User struct {
		Id    int64 `sql:",primary"`
		Name  string
		Proto *thunderpb.BinlogEvent
	}

	schema := NewSchema()
	schema.MustRegisterType("users", AutoIncrement, User{})

	db := NewDB(testDb.DB, schema)
	if _, err := db.InsertRow(context.Background(), &User{
		Name: "Bob",
		Proto: &thunderpb.BinlogEvent{
			Table: "foo",
		},
	}); err != nil {
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
			Proto: &thunderpb.BinlogEvent{
				Table: "foo",
			},
		},
	}); diff != "" {
		t.Errorf("diff: %s", diff)
	}
}
