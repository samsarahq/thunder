package livesql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"reflect"
	"sync"
	"time"

	"github.com/samsarahq/go/oops"
	"github.com/samsarahq/thunder/thunderpb"
	"github.com/siddontang/go-mysql/replication"
)

func makeCanonical(v interface{}) interface{} {
	switch v := v.(type) {
	case bool:
		return bool(v)
	case int8:
		return int64(v)
	case int16:
		return int64(v)
	case int32:
		return int64(v)
	case int64:
		return int64(v)
	case uint8:
		return uint64(v)
	case uint16:
		return uint64(v)
	case uint32:
		return uint64(v)
	case uint64:
		return uint64(v)
	case string:
		return string(v)
	case []byte:
		return []byte(v)
	case float32:
		return float64(v)
	case float64:
		return float64(v)
	case time.Time:
		return time.Time(v)
	default:
		return v
	}
}

func binlogValueToField(column interface{}) (*thunderpb.Field, error) {
	switch column := makeCanonical(column).(type) {
	case bool:
		return &thunderpb.Field{Kind: thunderpb.FieldKind_Bool, Bool: column}, nil
	case int64:
		return &thunderpb.Field{Kind: thunderpb.FieldKind_Int, Int: column}, nil
	case uint64:
		return &thunderpb.Field{Kind: thunderpb.FieldKind_Uint, Uint: column}, nil
	case string:
		return &thunderpb.Field{Kind: thunderpb.FieldKind_String, String_: column}, nil
	case []byte:
		return &thunderpb.Field{Kind: thunderpb.FieldKind_Bytes, Bytes: column}, nil
	case float64:
		return &thunderpb.Field{Kind: thunderpb.FieldKind_Float64, Float64: column}, nil
	case time.Time:
		return &thunderpb.Field{Kind: thunderpb.FieldKind_Time, Time: column}, nil
	default:
		return nil, fmt.Errorf("unknown type %s", reflect.TypeOf(column))
	}
}

func binlogRowToFields(binlogRow []interface{}, columns []string) (map[string]*thunderpb.Field, error) {
	if len(binlogRow) != len(columns) {
		return nil, oops.Errorf("unexpected number of fields: got %d, expected %d", len(binlogRow), len(columns))
	}

	fields := make(map[string]*thunderpb.Field, len(binlogRow))
	for i, value := range binlogRow {
		column := columns[i]
		field, err := binlogValueToField(value)
		if err != nil {
			return nil, oops.Errorf("bad binlog value %s: %s", column, err)
		}
		fields[columns[i]] = field
	}
	return fields, nil
}

type TableInfo struct {
	Version uint64
	Columns []string
}

type BinlogProxy struct {
	conn     *sql.DB
	database string

	syncer   *replication.BinlogSyncer
	streamer *replication.BinlogStreamer

	columns map[string][]string

	mu     sync.Mutex
	closed bool
}

// fetchColumns fetches the columns of a table
func (bp *BinlogProxy) fetchColumns(table string) ([]string, error) {
	rows, err := bp.conn.Query(`
		SELECT column_name
		FROM information_schema.columns
		WHERE table_schema = ? AND table_name = ?
		ORDER BY ordinal_position`, bp.database, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			return nil, err
		}
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return columns, nil
}

func (bp *BinlogProxy) getColumns(table string) ([]string, error) {
	if columns, ok := bp.columns[table]; ok {
		return columns, nil
	}

	columns, err := bp.fetchColumns(table)
	if err != nil {
		return nil, oops.Wrapf(err, "fetching columns")
	}

	bp.columns[table] = columns
	return columns, nil
}

// parseBinlogRowsEvent transforms a raw binlog rows event into an *update
//
// Because the binlog does not include a detailed table schema,
// parseBinlogRowsEvent uses the *DB to fetch the table's schema
func (bp *BinlogProxy) parseBinlogRowsEvent(event *replication.BinlogEvent) ([]*thunderpb.Change, error) {
	rowsEvent, ok := event.Event.(*replication.RowsEvent)
	if !ok {
		return nil, errors.New("event is not a rows event")
	}

	table := string(rowsEvent.Table.Table)

	columns, err := bp.getColumns(table)
	if err != nil {
		return nil, err
	}

	changes := make([]*thunderpb.Change, 0)

	// Parse rows into changes.
	switch event.Header.EventType {
	case replication.WRITE_ROWS_EVENTv1, replication.WRITE_ROWS_EVENTv2:
		for _, binlogRow := range rowsEvent.Rows {
			after, err := binlogRowToFields(binlogRow, columns)
			if err != nil {
				return nil, err
			}
			changes = append(changes, &thunderpb.Change{
				Table: table,
				After: after,
			})
		}

	case replication.UPDATE_ROWS_EVENTv1, replication.UPDATE_ROWS_EVENTv2:
		if len(rowsEvent.Rows)%2 != 0 {
			return nil, errors.New("expected even number of rows for update event")
		}

		for i := 0; i < len(rowsEvent.Rows); i += 2 {
			before, err := binlogRowToFields(rowsEvent.Rows[i], columns)
			if err != nil {
				return nil, err
			}
			after, err := binlogRowToFields(rowsEvent.Rows[i+1], columns)
			if err != nil {
				return nil, err
			}
			changes = append(changes, &thunderpb.Change{
				Table:  table,
				Before: before,
				After:  after,
			})
		}

	case replication.DELETE_ROWS_EVENTv1, replication.DELETE_ROWS_EVENTv2:
		for _, binlogRow := range rowsEvent.Rows {
			before, err := binlogRowToFields(binlogRow, columns)
			if err != nil {
				return nil, err
			}
			changes = append(changes, &thunderpb.Change{
				Table:  table,
				Before: before,
			})
		}

	default:
		return nil, fmt.Errorf("unknown event type %s", event.Header.EventType.String())
	}

	return changes, nil
}

// RunPollLoop is the core binlog function that fetches and distributes updates
// from MySQL
func (b *BinlogProxy) RunPollLoop() error {
	for {
		event, err := b.streamer.GetEvent(context.Background())
		if err != nil {
			b.mu.Lock()
			defer b.mu.Unlock()
			if b.closed {
				return nil
			}
			return err
		}

		switch inner := event.Event.(type) {
		case *replication.RowsEvent:
			if string(inner.Table.Schema) != b.database {
				continue
			}

			u, err := b.parseBinlogRowsEvent(event)
			if err == errNoDescriptor {
				continue
			} else if err != nil && err.Error() == "sql: database is closed" {
				continue
			} else if err != nil {
				// TODO: handle these errors more gracefully -- for now, we just log
				// them the console.
				log.Printf("error parsing rows event %v", err)
				continue
			}

			// b.tracker.processBinlog(u)
			for _, u := range u {
				log.Println(u)
			}

		case *replication.TableMapEvent:
			if string(inner.Schema) != b.database {
				continue
			}

			table := string(inner.Table)
			// According to the MySQL source, the TableID is unique for every
			// version of the table schema (though not persistent across server
			// restarts.)
			//
			// Whenever the table ID changes, the schema might have changed and we
			// flus the stored MySQL column information so we have the correct set
			// of columns ready before parsing.
			//
			// If an update quickly happens twice in a row, we might end up with a
			// set of columns that is too new. If the number of columns has
			// changed, we detect the error. If the number of columns stays the
			// same, we might return garbage data and miss invalidations.
			// Hopefully that happens rarely.
			delete(b.columns, table)
		}
	}
}

func (b *BinlogProxy) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	b.syncer.Close()
	return nil
}
