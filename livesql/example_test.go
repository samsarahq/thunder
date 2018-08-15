package livesql_test

import (
	"context"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/samsarahq/thunder/livesql"
	"github.com/samsarahq/thunder/reactive"
	"github.com/samsarahq/thunder/sqlgen"
	"github.com/samsarahq/thunder/testfixtures"
)

type Cat struct {
	Id   int64 `sql:",primary"`
	Name string
}

func Example() {
	config := testfixtures.DefaultDBConfig
	testDb, err := testfixtures.NewTestDatabase()
	if err != nil {
		panic(err)
	}
	defer testDb.Close()

	// Create users table.
	testDb.Exec("CREATE TABLE `cats` (`id` BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, `name` VARCHAR(100) NOT NULL);")

	// Register the User type in the schema.
	schema := sqlgen.NewSchema()
	schema.MustRegisterType("cats", sqlgen.AutoIncrement, Cat{})

	// Open a connection to the live DB.
	db, err := livesql.Open(config.Hostname, config.Port, config.Username, config.Password, testDb.DBName, schema)
	if err != nil {
		panic(err)
	}

	// Insert a dummy row.
	result, err := db.InsertRow(context.Background(), &Cat{Name: "Fluffles"})
	if err != nil {
		panic(err)
	}
	catId, err := result.LastInsertId()
	if err != nil {
		panic(err)
	}

	ch := make(chan struct{})
	// Wrap the query in a re-runner. This runner will re-trigger every time a change that would
	// affect queries inside of it being made.
	rerunner := reactive.NewRerunner(context.Background(), func(ctx context.Context) (interface{}, error) {
		user := &Cat{Id: catId}
		err := db.QueryRow(ctx, &user, sqlgen.Filter{}, nil)
		if err != nil {
			panic(err)
		}
		fmt.Println(user.Name)
		ch <- struct{}{}
		return nil, nil
	}, 200*time.Millisecond)
	defer rerunner.Stop()

	<-ch
	if err := db.UpdateRow(context.Background(), &Cat{Id: 1, Name: "Ruffles"}); err != nil {
		panic(err)
	}

	<-ch
	// Output:
	// Fluffles
	// Ruffles
}
