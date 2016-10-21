package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"

	"github.com/lib/pq"
)

const functionSource = `
    DECLARE 
        before json;
        after json;
        notification json;
    BEGIN
        IF (TG_OP = 'DELETE' OR TG_OP = 'UPDATE') THEN
            before = row_to_json(OLD);
        END IF;

        IF (TG_OP = 'INSERT' OR TG_OP = 'UPDATE') THEN
            after = row_to_json(NEW);
        END IF;
        
        notification = json_build_object(
            'table', TG_TABLE_NAME,
            'before', before,
            'after', after);
                        
        PERFORM pg_notify('changes', notification::text);

        RETURN NULL; 
    END;`

const selectFunction = `
    SELECT prosrc
    FROM pg_catalog.pg_namespace n
    JOIN pg_catalog.pg_proc ON pronamespace = n.oid
    WHERE nspname = 'public' AND proname = 'notify_change'`

const createOrReplaceFunction = `
    CREATE OR REPLACE FUNCTION notify_change() RETURNS TRIGGER AS $$` +
	functionSource +
	`$$ LANGUAGE plpgsql`

func maintainFunction(db *sql.DB) error {
	var source string
	if err := db.QueryRow(selectFunction).Scan(&source); err != nil {
		if err != sql.ErrNoRows {
			return fmt.Errorf("checking notify_change function failed: %s", err)
		}
	}

	if source != functionSource {
		if _, err := db.Exec(createOrReplaceFunction); err != nil {
			return fmt.Errorf("creating notify_change function failed: %s", err)
		}
	}

	return nil
}

const selectTriggers = `
    SELECT pg_class.relname
    FROM pg_trigger
    JOIN pg_class ON pg_class.oid = pg_trigger.tgrelid
    WHERE tgname = 'notify_change'`

const createTrigger = `
    CREATE TRIGGER notify_change
    AFTER INSERT OR UPDATE OR DELETE ON %s 
    FOR EACH ROW EXECUTE PROCEDURE notify_change()`

const dropTrigger = `DROP TRIGGER notify_change ON %s`

func maintainTriggers(db *sql.DB, tables []string) error {
	rows, err := db.Query(selectTriggers)
	if err != nil {
		return fmt.Errorf("checking triggers failed: %s", err)
	}

	found := make(map[string]bool)

	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return fmt.Errorf("checking triggers failed: %s", err)
		}
		found[table] = true
	}

	for _, table := range tables {
		if found[table] {
			delete(found, table)
			continue
		}
		if _, err := db.Exec(fmt.Sprintf(createTrigger, table)); err != nil {
			return fmt.Errorf("creating trigger failed: %s", err)
		}
	}

	for table := range found {
		if _, err := db.Exec(fmt.Sprintf(dropTrigger, table)); err != nil {
			return fmt.Errorf("dropping trigger failed: %s", err)
		}
	}

	return nil
}

type notification struct {
	Table  string                     `json:"table"`
	Before map[string]json.RawMessage `json:"before"`
	After  map[string]json.RawMessage `json:"after"`
}

func doit(db *sql.DB) {
	if err := maintainFunction(db); err != nil {
		panic(err)
	}
	if err := maintainTriggers(db, []string{"messages"}); err != nil {
		panic(err)
	}

	ch := make(chan *pq.Notification)

	dsn := ""

	listener, err := pq.NewListenerConn(dsn, ch)
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	if _, err := listener.Listen("changes"); err != nil {
		panic(err)
	}

	for message := range ch {
		var n notification
		if err := json.Unmarshal([]byte(message.Extra), &n); err != nil {
			panic(err)
		}
		log.Printf("%+v", n)
	}
}
