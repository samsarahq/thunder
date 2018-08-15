package livesql_test

import (
	"context"
	"testing"
	"time"

	"github.com/samsarahq/thunder/livesql"
	"github.com/samsarahq/thunder/reactive"
	"github.com/samsarahq/thunder/sqlgen"
	"github.com/samsarahq/thunder/testfixtures"
	"github.com/stretchr/testify/assert"
)

type User struct {
	Id   int64 `sql:",primary"`
	Name string
	Uuid testfixtures.CustomType
	Mood *testfixtures.CustomType
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
                       mood VARCHAR(255)
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
	}, 50*time.Millisecond)
	defer rerunner.Stop()

	// Initial rerunner query matches initial insert.
	assert.Equal(t, User{Id: userId, Name: name, Uuid: uuid, Mood: &mood}, <-users)

	// Upon update we get another rerunner result with the updated name.
	err = db.UpdateRow(context.Background(), &User{Id: userId, Name: newName, Uuid: uuid, Mood: &mood})
	assert.NoError(t, err)
	assert.Equal(t, User{Id: userId, Name: newName, Uuid: uuid, Mood: &mood}, <-users)

	// Upon update we get another rerunner result with the updated uuid.
	err = db.UpdateRow(context.Background(), &User{Id: userId, Name: newName, Uuid: newUuid, Mood: &mood})
	assert.NoError(t, err)
	assert.Equal(t, User{Id: userId, Name: newName, Uuid: newUuid, Mood: &mood}, <-users)

	// Upon update we get another rerunner result with the updated mood.
	err = db.UpdateRow(context.Background(), &User{Id: userId, Name: newName, Uuid: newUuid, Mood: &newMood})
	assert.NoError(t, err)
	assert.Equal(t, User{Id: userId, Name: newName, Uuid: newUuid, Mood: &newMood}, <-users)
}
