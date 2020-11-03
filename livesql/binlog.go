package livesql

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/northvolt/thunder/internal/fields"
	"github.com/northvolt/thunder/logger"
	"github.com/northvolt/thunder/sqlgen"
	"github.com/siddontang/go-mysql/mysql"
	"github.com/siddontang/go-mysql/replication"
)

// Binlog streams the MySQL binary replication log, parses it, and broadcasts
// updates
type Binlog struct {
	db      *sqlgen.DB
	tracker *dbTracker

	database string

	syncer   *replication.BinlogSyncer
	streamer *replication.BinlogStreamer

	tableVersions map[string]uint64

	delayMu sync.Mutex
	delay   time.Duration

	mu         sync.Mutex
	columnMaps map[string]*columnMap
	closed     bool

	logger logger.Logger
}

// checkVariable checks that the requested MySQL global variable matches
// an expected configuration
func checkVariable(conn *sql.DB, variable, expected string) error {
	row := conn.QueryRow(fmt.Sprintf(`SHOW GLOBAL VARIABLES LIKE "%s"`, variable))
	var value string
	var ignored interface{}
	if err := row.Scan(&ignored, &value); err != nil {
		return fmt.Errorf("error reading MySQL variable %s: %s", variable, err)
	}

	if !strings.EqualFold(value, expected) {
		return fmt.Errorf("expected MySQL variable %s to be %s, but got %s", variable, expected, value)
	}

	return nil
}

// getPosition fetches the current MySQL binlog position
func getPosition(conn *sql.DB) (mysql.Position, error) {
	row := conn.QueryRow("SHOW MASTER STATUS")
	var position mysql.Position
	var ignored interface{}
	if err := row.Scan(&position.Name, &position.Pos, &ignored, &ignored, &ignored); err != nil {
		return mysql.Position{}, fmt.Errorf("error retrieving MySQL binlog position: %s", err)
	}
	return position, nil
}

// NewBinlog constructs a new Binlog for a given DB.
func NewBinlog(ldb *LiveDB, host string, port uint16, username, password, database string) (*Binlog, error) {
	return NewBinlogWithSource(ldb, ldb.Conn, host, port, username, password, database)
}

// NewBinlogWithSource constructs a new Binlog for a given DB and a source DB
//
// NewBinlogWithSource verifies that the given DB has been correctly configured for
// streaming changes.
func NewBinlogWithSource(ldb *LiveDB, sourceDB *sql.DB, host string, port uint16, username, password, database string) (*Binlog, error) {
	db := ldb.DB
	tracker := ldb.tracker

	if err := checkVariable(sourceDB, "binlog_format", "ROW"); err != nil {
		return nil, err
	}
	if err := checkVariable(sourceDB, "binlog_row_image", "FULL"); err != nil {
		return nil, err
	}

	position, err := getPosition(sourceDB)
	if err != nil {
		return nil, err
	}

	slaveId := make([]byte, 4)
	if _, err := rand.Read(slaveId); err != nil {
		return nil, err
	}

	// Slave hosts have a max hostname length of 60 characters.
	const maxHostNameLength = 60
	localHostName, err := os.Hostname()
	if err != nil {
		localHostName = "localhost"
	}
	if len(localHostName) > maxHostNameLength {
		runes := []rune(localHostName)
		localHostName = string(runes[0:maxHostNameLength])
	}

	syncer := replication.NewBinlogSyncer(&replication.BinlogSyncerConfig{
		ServerID:  binary.LittleEndian.Uint32(slaveId),
		Host:      host,
		Localhost: localHostName,
		Port:      port,
		User:      username,
		Password:  password,
	})

	streamer, err := syncer.StartSync(position)
	if err != nil {
		syncer.Close()
		return nil, err
	}

	return &Binlog{
		db: db,

		database: database,

		tracker:       tracker,
		syncer:        syncer,
		streamer:      streamer,
		tableVersions: make(map[string]uint64),
		columnMaps:    make(map[string]*columnMap),

		logger: logger.New(),
	}, nil
}

// columnMap stores a column permutation indices
type columnMap struct {
	expectedColumns int
	source          []int
}

// fetchColumns fetches the columns of a table
func fetchColumns(conn *sql.DB, database string, table string) ([]string, error) {
	rows, err := conn.Query(`
		SELECT column_name
		FROM information_schema.columns
		WHERE table_schema = ? AND table_name = ?
		ORDER BY ordinal_position`, database, table)
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

// buildColumnMap constructs a columnMap from column information fetched from
// the database
func buildColumnMap(conn *sql.DB, database string, table *sqlgen.Table) (*columnMap, error) {
	columns, err := fetchColumns(conn, database, table.Name)
	if err != nil {
		return nil, err
	}

	columnMap := &columnMap{
		expectedColumns: len(columns),
	}

	columnIndex := make(map[string]int)
	for i, column := range columns {
		columnIndex[column] = i
	}

	for _, column := range table.Columns {
		if idx, ok := columnIndex[column.Name]; ok {
			columnMap.source = append(columnMap.source, idx)
		} else {
			columnMap.source = append(columnMap.source, -1)
		}
	}

	return columnMap, nil
}

// parseBinlogRow parses a binlog row into a struct
func parseBinlogRow(table *sqlgen.Table, binlogRow []interface{}, columnMap *columnMap) (interface{}, error) {
	ptr := reflect.New(table.Type)
	elem := ptr.Elem()

	if len(binlogRow) != columnMap.expectedColumns {
		return nil, fmt.Errorf("binlog for %s has %d columns, expected %d",
			table.Name, len(binlogRow), columnMap.expectedColumns)
	}

	scanners := table.Scanners.Get().([]interface{})
	defer table.Scanners.Put(scanners)

	for i, j := range columnMap.source {
		if j == -1 {
			continue
		}
		field := elem.FieldByIndex(table.Columns[i].Index)
		if field.Kind() != reflect.Ptr {
			field = field.Addr()
		}
		scanner := scanners[i].(*fields.Scanner)
		scanner.Target(field)
		if err := scanner.Scan(binlogRow[j]); err != nil {
			return nil, fmt.Errorf("binlog: `%s`.`%s` error: %v", table.Name, table.Columns[i].Name, err)
		}
	}

	return ptr.Interface(), nil
}

// getColumnMap returns the a column map for the table, fetching schema
// information if necessary
func (b *Binlog) getColumnMap(table *sqlgen.Table) (*columnMap, error) {
	if columnMap, ok := b.columnMaps[table.Name]; ok {
		return columnMap, nil
	}

	columnMap, err := buildColumnMap(b.db.Conn, b.database, table)
	if err != nil {
		return nil, err
	}

	b.columnMaps[table.Name] = columnMap
	return columnMap, nil
}

var errNoDescriptor = errors.New("no known descriptor")

// delta represents an update to a SQL row
//
// before or after are nil if the row was newly added or deleted
type delta struct {
	key           string
	before, after interface{}
}

// update holds a set of updates to a SQL table
//
// If the binlog had trouble parsing the update, err will be non-nil.
type update struct {
	table  string
	deltas []delta
	err    error
}

// delayedUpdate holds an update and the earlist timestamp the update can be applied.
type delayedUpdate struct {
	*update
	applyAfter time.Time
}

// parseBinlogRowsEvent transforms a raw binlog rows event into an *update
//
// Because the binlog does not include a detailed table schema,
// parseBinlogRowsEvent uses the *DB to fetch the table's schema
func (b *Binlog) parseBinlogRowsEvent(event *replication.BinlogEvent) (*update, error) {
	rowsEvent, ok := event.Event.(*replication.RowsEvent)
	if !ok {
		return nil, errors.New("event is not a rows event")
	}

	update := &update{
		table: string(rowsEvent.Table.Table),
	}

	schema, ok := b.db.Schema.ByName[update.table]
	if !ok {
		return nil, errNoDescriptor
	}

	columnMap, err := b.getColumnMap(schema)
	if err != nil {
		return nil, err
	}

	// Transform rows into deltas:
	switch event.Header.EventType {
	case replication.WRITE_ROWS_EVENTv1, replication.WRITE_ROWS_EVENTv2:
		for _, binlogRow := range rowsEvent.Rows {
			fields, err := parseBinlogRow(schema, binlogRow, columnMap)
			if err != nil {
				return nil, err
			}
			update.deltas = append(update.deltas, delta{after: fields})
		}

	case replication.UPDATE_ROWS_EVENTv1, replication.UPDATE_ROWS_EVENTv2:
		if len(rowsEvent.Rows)%2 != 0 {
			return nil, errors.New("expected even number of rows for update event")
		}

		for i := 0; i < len(rowsEvent.Rows); i += 2 {
			before, err := parseBinlogRow(schema, rowsEvent.Rows[i], columnMap)
			if err != nil {
				return nil, err
			}
			after, err := parseBinlogRow(schema, rowsEvent.Rows[i+1], columnMap)
			if err != nil {
				return nil, err
			}
			update.deltas = append(update.deltas, delta{before: before, after: after})
		}

	case replication.DELETE_ROWS_EVENTv1, replication.DELETE_ROWS_EVENTv2:
		for _, binlogRow := range rowsEvent.Rows {
			fields, err := parseBinlogRow(schema, binlogRow, columnMap)
			if err != nil {
				return nil, err
			}
			update.deltas = append(update.deltas, delta{before: fields})
		}

	default:
		return nil, fmt.Errorf("unknown event type %s", event.Header.EventType.String())
	}

	return update, nil
}

// SetOutput sets the destination for the error logger.
func (b *Binlog) SetLogger(l logger.Logger) {
	b.logger = l
}

// SetUpdateDelay sets the duration by which future updates will be delayed.
func (b *Binlog) SetUpdateDelay(d time.Duration) {
	b.delayMu.Lock()
	defer b.delayMu.Unlock()
	b.delay = d
}

// RunPollLoop is the core binlog function that fetches and distributes updates
// from MySQL
func (b *Binlog) RunPollLoop() error {
	updateCh := make(chan delayedUpdate, 1024)
	defer close(updateCh)

	go func() {
		for du := range updateCh {
			time.Sleep(du.applyAfter.Sub(time.Now()))
			b.tracker.processBinlog(du.update)
		}
	}()

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
				b.logger.Error("livesql: failed to parse rows event", "error", err)
				continue
			}

			b.delayMu.Lock()
			delay := b.delay
			b.delayMu.Unlock()

			updateCh <- delayedUpdate{update: u, applyAfter: time.Now().Add(delay)}

		case *replication.TableMapEvent:
			if string(inner.Schema) != b.database {
				continue
			}

			table := string(inner.Table)
			if version, found := b.tableVersions[table]; !found || version != inner.TableID {
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
				b.tableVersions[table] = inner.TableID
				delete(b.columnMaps, table)
			}
		}
	}
}

func (b *Binlog) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	b.syncer.Close()
	return nil
}
