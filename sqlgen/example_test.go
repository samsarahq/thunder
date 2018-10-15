package sqlgen_test

import (
	"context"
	"fmt"
	"net"
	"reflect"

	"github.com/samsarahq/thunder/internal/testfixtures"
	"github.com/samsarahq/thunder/sqlgen"
	uuid "github.com/satori/go.uuid"
)

type UUID uuid.UUID

func (u UUID) MarshalBinary() ([]byte, error) {
	return u[:], nil
}

func (u *UUID) UnmarshalBinary(b []byte) error {
	copy(u[:], b)
	return nil
}

// You can override how values are serialized via struct tags. The following tags are available:
//
// string (encoding.TextMarshaler, encoding.TextUnmarshaler), binary (encoding.BinaryMarshaler,
// encoding.BinaryUnmarshaler, gogoproto.Marshaler, gogoproto.Unmarshaler), json (json.Marshaler,
// json.Unmarshaler).
func Example_tagOverride() {
	testDb, _ := testfixtures.NewTestDatabase()
	defer testDb.Close()

	_, err := testDb.Exec(`
		CREATE TABLE users (
			uuid          BINARY(16) PRIMARY KEY,
			name          VARCHAR(255),
			ip            VARCHAR(255),
			configuration BLOB
		)
	`)
	if err != nil {
		panic(err)
	}

	ctx := context.TODO()

	type User struct {
		UUID          UUID `sql:"uuid,primary,binary"`
		Name          string
		IP            net.IP                 `sql:"ip,string"`
		Configuration map[string]interface{} `sql:"configuration,json"`
	}

	schema := sqlgen.NewSchema()
	schema.MustRegisterType("users", sqlgen.UniqueId, User{})

	db := sqlgen.NewDB(testDb.DB, schema)
	uuid, _ := uuid.NewV4()

	initialUser := &User{
		UUID: UUID(uuid),             // => BINARY (via uuid.MarshalBinary())
		Name: "Jean",                 // => VARCHAR
		IP:   net.IPv4(127, 0, 0, 1), // => VARCHAR (via ip.MarshalText())
		Configuration: map[string]interface{}{ // => JSON (via json.Marshal(configuration))
			"darkmode": true,
		},
	}
	_, err = db.InsertRow(ctx, initialUser)
	if err != nil {
		panic(err)
	}

	user := &User{UUID: UUID(uuid)}
	err = db.QueryRow(ctx, &user, nil, nil)
	if err != nil {
		panic(err)
	}

	// Output: They match!
	if reflect.DeepEqual(*user, *initialUser) {
		fmt.Println("They match!")
	}
}
