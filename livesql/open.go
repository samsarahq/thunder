package livesql

import (
	"database/sql"
	"fmt"

	"github.com/obad2015/thunder/sqlgen"
)

func Open(hostname string, port uint16, username, password, database string, schema *sqlgen.Schema) (*LiveDB, error) {
	db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", username, password, hostname, port, database))
	if err != nil {
		return nil, err
	}

	sqlgenDB := sqlgen.NewDB(db, schema)
	liveDB := NewLiveDB(sqlgenDB)

	binlog, err := NewBinlog(liveDB, hostname, port, username, password, database)
	if err != nil {
		db.Close()
		return nil, err
	}

	go func() {
		defer binlog.Close()
		if err := binlog.RunPollLoop(); err != nil {
			panic(err)
		}
	}()

	return liveDB, nil
}
