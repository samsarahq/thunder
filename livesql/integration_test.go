package livesql_test

import (
	"context"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/samsarahq/thunder/batch"
	"github.com/samsarahq/thunder/internal/testfixtures"
	"github.com/samsarahq/thunder/livesql"
	"github.com/samsarahq/thunder/reactive"
	"github.com/samsarahq/thunder/sqlgen"
	"github.com/stretchr/testify/assert"
)

type User struct {
	Id              int64 `sql:",primary"`
	Name            string
	Uuid            testfixtures.CustomType
	Mood            *testfixtures.CustomType
	ImplicitNullStr string `sql:,implicitnull`
}

func TestIntegrationBasic(t *testing.T) {
	config := testfixtures.DefaultDBConfig
	schema := sqlgen.NewSchema()
	schema.MustRegisterType("users", sqlgen.AutoIncrement, User{})
	testDb, err := testfixtures.NewTestDatabase()
	if err != nil {
		t.Fatal(err)
	}
	defer testDb.Close()

	_, err = testDb.DB.Exec(`
               CREATE TABLE users (
                       id   BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
                       name VARCHAR(255),
                       uuid VARCHAR(255),
                       mood VARCHAR(255),
                       implicit_null_str VARCHAR(255)
               )
       `)
	if err != nil {
		t.Fatal(err)
	}

	db, err := livesql.Open(config.Hostname, config.Port, config.Username, config.Password, testDb.DBName, schema)
	if err != nil {
		t.Fatal(err)
	}

	// Values
	name := "Jean"
	newName := "Joan"
	mood := testfixtures.CustomTypeFromString("freeform")
	newMood := testfixtures.CustomTypeFromString("outrage")
	uuid := testfixtures.CustomTypeFromString("11238491293")
	newUuid := testfixtures.CustomTypeFromString("11232481203")
	implicitNullStr := ""
	newImplicitNullStr := "not null"

	result, err := db.InsertRow(context.Background(), &User{Name: name, Uuid: uuid, Mood: &mood})
	assert.NoError(t, err)
	userId, err := result.LastInsertId()
	assert.NoError(t, err)

	// We use a channel to pass around query updates for testing.
	users := make(chan User)
	rerunner := reactive.NewRerunner(context.Background(), func(ctx context.Context) (interface{}, error) {
		user := &User{Id: userId}
		if err := db.QueryRow(ctx, &user, sqlgen.Filter{}, nil); err != nil {
			t.Errorf("coudln't query row: %v", err)
		}
		users <- *user
		return nil, nil
	}, 50*time.Millisecond, false)
	defer rerunner.Stop()

	// Initial rerunner query matches initial insert.
	assert.Equal(t, User{Id: userId, Name: name, Uuid: uuid, Mood: &mood, ImplicitNullStr: implicitNullStr}, <-users)

	// Upon update we get another rerunner result with the updated name.
	err = db.UpdateRow(context.Background(), &User{Id: userId, Name: newName, Uuid: uuid, Mood: &mood, ImplicitNullStr: implicitNullStr})
	assert.NoError(t, err)
	assert.Equal(t, User{Id: userId, Name: newName, Uuid: uuid, Mood: &mood, ImplicitNullStr: implicitNullStr}, <-users)

	// Upon update we get another rerunner result with the updated uuid.
	err = db.UpdateRow(context.Background(), &User{Id: userId, Name: newName, Uuid: newUuid, Mood: &mood, ImplicitNullStr: implicitNullStr})
	assert.NoError(t, err)
	assert.Equal(t, User{Id: userId, Name: newName, Uuid: newUuid, Mood: &mood, ImplicitNullStr: implicitNullStr}, <-users)

	// Upon update we get another rerunner result with the updated mood.
	err = db.UpdateRow(context.Background(), &User{Id: userId, Name: newName, Uuid: newUuid, Mood: &newMood, ImplicitNullStr: implicitNullStr})
	assert.NoError(t, err)
	assert.Equal(t, User{Id: userId, Name: newName, Uuid: newUuid, Mood: &newMood, ImplicitNullStr: implicitNullStr}, <-users)

	// Upon update we get another rerunner result with the updated implicitNullStr.
	err = db.UpdateRow(context.Background(), &User{Id: userId, Name: newName, Uuid: newUuid, Mood: &newMood, ImplicitNullStr: newImplicitNullStr})
	assert.NoError(t, err)
	assert.Equal(t, User{Id: userId, Name: newName, Uuid: newUuid, Mood: &newMood, ImplicitNullStr: newImplicitNullStr}, <-users)

	// Upon update we get another rerunner result with the original implicitNullStr.
	err = db.UpdateRow(context.Background(), &User{Id: userId, Name: newName, Uuid: newUuid, Mood: &newMood, ImplicitNullStr: implicitNullStr})
	assert.NoError(t, err)
	assert.Equal(t, User{Id: userId, Name: newName, Uuid: newUuid, Mood: &newMood, ImplicitNullStr: implicitNullStr}, <-users)
}

func TestIntegrationFilterCustomType(t *testing.T) {
	config := testfixtures.DefaultDBConfig
	schema := sqlgen.NewSchema()
	schema.MustRegisterType("users", sqlgen.AutoIncrement, User{})
	testDb, err := testfixtures.NewTestDatabase()
	if err != nil {
		t.Fatal(err)
	}
	defer testDb.Close()

	_, err = testDb.DB.Exec(`
               CREATE TABLE users (
                       id   BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
                       name VARCHAR(255),
                       uuid VARCHAR(255),
                       mood VARCHAR(255),
                       implicit_null_str VARCHAR(255)
               )
       `)
	if err != nil {
		t.Fatal(err)
	}

	db, err := livesql.Open(config.Hostname, config.Port, config.Username, config.Password, testDb.DBName, schema)
	if err != nil {
		t.Fatal(err)
	}

	// Values
	name := "Jean"
	newName := "Joan"
	mood := testfixtures.CustomTypeFromString("freeform")
	newMood := testfixtures.CustomTypeFromString("outrage")
	uuid := testfixtures.CustomTypeFromString("11238491293")
	newUuid := testfixtures.CustomTypeFromString("11232481203")
	implicitNullStr := ""
	newImplicitNullStr := "not null"

	result, err := db.InsertRow(context.Background(), &User{Name: name, Uuid: uuid, Mood: &mood, ImplicitNullStr: implicitNullStr})
	assert.NoError(t, err)
	userId, err := result.LastInsertId()
	assert.NoError(t, err)

	// We use a channel to pass around query updates for testing.
	users := make(chan *User)
	ctx := batch.WithBatching(context.Background())
	rerunner := reactive.NewRerunner(ctx, func(ctx context.Context) (interface{}, error) {
		user := &User{}
		if err := db.QueryRow(ctx, &user, sqlgen.Filter{"mood": &mood}, nil); err != nil {
			users <- nil
			return nil, nil
		}
		users <- user
		return nil, nil
	}, 50*time.Millisecond, false)
	defer rerunner.Stop()

	// Initial rerunner query matches initial insert.
	assert.Equal(t, &User{Id: userId, Name: name, Uuid: uuid, Mood: &mood, ImplicitNullStr: implicitNullStr}, <-users)

	// Upon update we get another rerunner result with the updated name.
	err = db.UpdateRow(context.Background(), &User{Id: userId, Name: newName, Uuid: uuid, Mood: &mood, ImplicitNullStr: implicitNullStr})
	assert.NoError(t, err)
	assert.Equal(t, &User{Id: userId, Name: newName, Uuid: uuid, Mood: &mood, ImplicitNullStr: implicitNullStr}, <-users)

	// Upon update we get another rerunner result with the updated uuid.
	err = db.UpdateRow(context.Background(), &User{Id: userId, Name: newName, Uuid: newUuid, Mood: &mood, ImplicitNullStr: implicitNullStr})
	assert.NoError(t, err)
	assert.Equal(t, &User{Id: userId, Name: newName, Uuid: newUuid, Mood: &mood, ImplicitNullStr: implicitNullStr}, <-users)

	// Upon update we get another rerunner result with the updated implicitNullStr.
	err = db.UpdateRow(context.Background(), &User{Id: userId, Name: newName, Uuid: newUuid, Mood: &mood, ImplicitNullStr: newImplicitNullStr})
	assert.NoError(t, err)
	assert.Equal(t, &User{Id: userId, Name: newName, Uuid: newUuid, Mood: &mood, ImplicitNullStr: newImplicitNullStr}, <-users)

	// Upon update we get another rerunner result with the updated mood.
	err = db.UpdateRow(context.Background(), &User{Id: userId, Name: newName, Uuid: newUuid, Mood: &newMood, ImplicitNullStr: implicitNullStr})
	assert.NoError(t, err)
	assert.Equal(t, (*User)(nil), <-users)
}
