package sqlgen

import (
	"bytes"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/samsarahq/thunder/internal/fields"
)

type Filter map[string]interface{}

type SelectOptions struct {
	Where  string
	Values []interface{}

	OrderBy string
	Limit   int
}

func (s *SelectOptions) IncludeFilter(table *Table, filter Filter) error {
	simpleWhere, err := makeWhere(table, filter)
	if err != nil {
		return err
	}
	filterWhere, filterValues := simpleWhere.ToSQL()

	if filterWhere != "" {
		if s.Where != "" {
			s.Where = fmt.Sprintf("(%s) AND (%s)", filterWhere, s.Where)
			s.Values = append(filterValues, s.Values...)
		} else {
			s.Where, s.Values = filterWhere, filterValues
		}
	}

	return nil
}

type PrimaryKeyType int

const (
	AutoIncrement PrimaryKeyType = iota
	UniqueId
)

// parseQueryRow parses a row from a sql.DB query into a struct
func parseQueryRow(table *Table, scanner *sql.Rows) (interface{}, error) {
	ptr := reflect.New(table.Type)
	elem := ptr.Elem()

	scanners := table.Scanners.Get().([]interface{})
	defer table.Scanners.Put(scanners)

	// Descriptor Scanner is instantiated with a reference to our struct fields.
	// It scans directly into our struct.
	for i, column := range table.Columns {
		field := elem.FieldByIndex(column.Index)
		if field.Kind() != reflect.Ptr {
			field = field.Addr()
		}
		// Scan into field.
		scanners[i].(*fields.Scanner).Target(field)
	}

	if err := scanner.Scan(scanners...); err != nil {
		columns, _ := scanner.Columns()
		return nil, fmt.Errorf("sqlgen: parsing error for `%s`.(%v): %v", table.Name, columns, err)
	}

	return ptr.Interface(), nil
}

func CopySlice(result interface{}, rows []interface{}) error {
	ptr := reflect.ValueOf(result)
	slice := ptr.Elem()
	slice.Set(reflect.MakeSlice(slice.Type(), len(rows), len(rows)))
	for i, row := range rows {
		if row != nil {
			slice.Index(i).Set(reflect.ValueOf(row))
		}
	}

	return nil
}

func CopySingletonSlice(result interface{}, rows []interface{}) error {
	ptr := reflect.ValueOf(result)

	switch len(rows) {
	case 0:
		return sql.ErrNoRows
	case 1:
		ptr.Elem().Set(reflect.ValueOf(rows[0]))
	default:
		return errors.New("expected no more than 1 result")
	}
	return nil
}

// makeSnake converts a CamelCase identifier into its snake_case equivalent
func makeSnake(s string) string {
	var b bytes.Buffer
	for i, c := range s {
		if i > 0 && unicode.IsUpper(c) {
			b.WriteRune('_')
		}
		b.WriteRune(unicode.ToLower(c))
	}
	return b.String()
}

type Column struct {
	Name    string
	Primary bool

	Descriptor *fields.Descriptor

	Index []int
	Order int
}

type Table struct {
	Name           string
	Type           reflect.Type
	PrimaryKeyType PrimaryKeyType

	Columns       []*Column
	ColumnsByName map[string]*Column

	Scanners *sync.Pool
}

func (s *Schema) buildDescriptor(table string, primaryKeyType PrimaryKeyType, typ reflect.Type) (*Table, error) {
	if typ.Kind() != reflect.Struct {
		return nil, fmt.Errorf("bad type %s: not a struct", typ)
	}

	var columns []*Column
	columnsByName := make(map[string]*Column)

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath != "" {
			continue
		}
		if field.Anonymous {
			return nil, fmt.Errorf("bad type %s: anonymous fields not supported", typ)
		}

		tags := strings.Split(field.Tag.Get("sql"), ",")
		var column string
		if len(tags) > 0 {
			column = tags[0]
		}
		if column == "" {
			column = makeSnake(field.Name)
		}
		if column == "-" {
			continue
		}

		primary := false

		if len(tags) > 1 {
			for _, tag := range tags[1:] {
				switch tag {
				case "primary":
					primary = true
				case "binary", "json", "string":
					// Do nothing, fields will handle these.
				case "implicitnull":
					if field.Type.Kind() == reflect.Ptr {
						return nil, fmt.Errorf("bad type %s: column %s cannot use `implicitnull` with pointer type", typ, column)
					}
				default:
					return nil, fmt.Errorf("bad type %s: column %s has unexpected tag %s", typ, column, tag)
				}
			}
		}

		if _, ok := columnsByName[column]; ok {
			return nil, fmt.Errorf("bad type %s: duplicate column %s", typ, column)
		}

		d := fields.New(field.Type, tags[1:])
		if err := d.ValidateSQLType(); err != nil {
			return nil, fmt.Errorf("bad type %s: %s %v", typ, column, err)
		}

		descriptor := &Column{
			Name:    column,
			Primary: primary,

			Index: field.Index,
			Order: len(columns),

			Descriptor: d,
		}

		columns = append(columns, descriptor)
		columnsByName[column] = descriptor
	}

	hasPrimary := false
	for _, column := range columns {
		if column.Primary {
			hasPrimary = true
			break
		}
	}
	if !hasPrimary {
		return nil, fmt.Errorf("bad type %s: no primary key specified", typ)
	}

	scanners := &sync.Pool{
		New: func() interface{} {
			scanners := make([]interface{}, len(columns))
			for i, column := range columns {
				scanners[i] = column.Descriptor.Scanner()
			}
			return scanners
		},
	}

	return &Table{
		Name:           table,
		Type:           typ,
		PrimaryKeyType: primaryKeyType,

		Columns:       columns,
		ColumnsByName: columnsByName,

		Scanners: scanners,
	}, nil
}

// unbuildStruct extracts SQL values from a struct.
func (table *Table) unbuildStruct(strct interface{}) ([]interface{}, error) {
	elem := reflect.ValueOf(strct).Elem()
	values := make([]interface{}, len(table.Columns))

	for i, column := range table.Columns {
		val := elem.FieldByIndex(column.Index)
		var err error
		values[i], err = column.Descriptor.Valuer(val).Value()
		if err != nil {
			return nil, fmt.Errorf("sqlgen: serialization error for `%s`.`%s`: %v", table.Name, column.Name, err)
		}
	}

	return values, nil
}

type Schema struct {
	ByName map[string]*Table
	ByType map[reflect.Type]*Table
}

func NewSchema() *Schema {
	return &Schema{
		ByName: make(map[string]*Table),
		ByType: make(map[reflect.Type]*Table),
	}
}

func (s *Schema) RegisterType(table string, primaryKeyType PrimaryKeyType, value interface{}) error {
	if _, ok := s.ByName[table]; ok {
		return fmt.Errorf("table %s registered twice", table)
	}
	typ := reflect.TypeOf(value)
	if _, ok := s.ByType[typ]; ok {
		return fmt.Errorf("type %s registered twice", typ)
	}

	descriptor, err := s.buildDescriptor(table, primaryKeyType, typ)
	if err != nil {
		return err
	}

	s.ByName[table] = descriptor
	s.ByType[typ] = descriptor
	return nil
}

func (s *Schema) MustRegisterType(table string, primaryKeyType PrimaryKeyType, value interface{}) {
	if err := s.RegisterType(table, primaryKeyType, value); err != nil {
		panic(err)
	}
}

func (s *Schema) get(typ reflect.Type) (*Table, error) {
	table, ok := s.ByType[typ]
	if !ok {
		return nil, fmt.Errorf("Type %s not in schema. Make sure to add new schema to models/sqlgen.go.", typ)
	}
	return table, nil
}

func (s *Schema) ParseRows(query *SelectQuery, res *sql.Rows) ([]interface{}, error) {
	table, ok := s.ByName[query.Table]
	if !ok {
		return nil, errors.New("unknown table")
	}

	var rows []interface{}
	for res.Next() {
		row, err := parseQueryRow(table, res)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	if err := res.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

// whereElem is a sortable part of a WHERE clause used to build
// deterministically-ordered WHERE clauses
type whereElem struct {
	column *Column
	value  interface{}
}

// whereElemsByIndex sorts whereElems by column order
type whereElemsByIndex []whereElem

func (l whereElemsByIndex) Len() int           { return len(l) }
func (l whereElemsByIndex) Less(a, b int) bool { return l[a].column.Order < l[b].column.Order }
func (l whereElemsByIndex) Swap(a, b int)      { l[a], l[b] = l[b], l[a] }

// makeWhere builds a new SimpleWhere for table from filter
func makeWhere(table *Table, filter Filter) (*SimpleWhere, error) {
	var l whereElemsByIndex

	for name, value := range filter {
		column, ok := table.ColumnsByName[name]
		if !ok {
			return nil, fmt.Errorf("unknown column %s", name)
		}

		v, err := column.Descriptor.Valuer(reflect.ValueOf(value)).Value()
		if err != nil {
			return nil, fmt.Errorf("sqlgen: filter error for `%s`.`%s`: %v", table.Name, column.Name, err)
		}
		l = append(l, whereElem{column: column, value: v})
	}

	sort.Sort(l)
	columns := []string{}
	values := []interface{}{}
	for _, elem := range l {
		columns = append(columns, elem.column.Name)
		values = append(values, elem.value)
	}

	return &SimpleWhere{
		Columns: columns,
		Values:  values,
	}, nil
}

type baseCountQuery struct {
	Table  *Table
	Filter Filter
}

func (b *baseCountQuery) makeCountQuery() (*countQuery, error) {
	where, err := makeWhere(b.Table, b.Filter)
	if err != nil {
		return nil, err
	}

	return &countQuery{
		Table:   b.Table.Name,
		Where:   where,
	}, nil
}

var errBadCountModelType = errors.New("count model value should be a pointer to a struct")

func checkCountModelTypeShape(typ reflect.Type) (reflect.Type, error) {
	if typ.Kind() != reflect.Ptr {
		return nil, errBadCountModelType
	}
	typ = typ.Elem()
	if typ.Kind() != reflect.Struct {
		return nil, errBadCountModelType
	}
	return typ, nil
}

func (s *Schema) makeCount(model interface{}, filter Filter) (*baseCountQuery, error) {
	ptr := reflect.ValueOf(model)
	typ, err := checkCountModelTypeShape(ptr.Type())
	if err != nil {
		return nil, err
	}

	table, err := s.get(typ)
	if err != nil {
		return nil, err
	}

	return &baseCountQuery{
		Table:  table,
		Filter: filter,
	}, nil
}

type BaseSelectQuery struct {
	Table   *Table
	Filter  Filter
	Options *SelectOptions
}

func (b *BaseSelectQuery) MakeSelectQuery() (*SelectQuery, error) {
	var columns []string
	for _, column := range b.Table.Columns {
		columns = append(columns, column.Name)
	}

	options := b.Options
	if options == nil {
		options = &SelectOptions{}
	}
	// XXX: This assumes a BaseSelectQuery is only used once, as it modifies the
	// options struct. That's true for now, but should be cleaned up.
	if err := options.IncludeFilter(b.Table, b.Filter); err != nil {
		return nil, err
	}

	return &SelectQuery{
		Table:   b.Table.Name,
		Columns: columns,
		Options: options,
	}, nil
}

// makeSelect builds a new BaseQuery for table with filter
func (s *Schema) makeSelect(typ reflect.Type, filter Filter, options *SelectOptions) (*BaseSelectQuery, error) {
	table, err := s.get(typ)
	if err != nil {
		return nil, err
	}

	return &BaseSelectQuery{
		Table:   table,
		Filter:  filter,
		Options: options,
	}, nil
}

var errBadQueryType = errors.New("query result should be a pointer to a slice of pointers to struct")

func checkQueryTypeShape(typ reflect.Type) (reflect.Type, error) {
	if typ.Kind() != reflect.Ptr {
		return nil, errBadQueryType
	}
	typ = typ.Elem()
	if typ.Kind() != reflect.Slice {
		return nil, errBadQueryType
	}
	typ = typ.Elem()
	if typ.Kind() != reflect.Ptr {
		return nil, errBadQueryType
	}
	typ = typ.Elem()
	if typ.Kind() != reflect.Struct {
		return nil, errBadQueryType
	}
	return typ, nil
}

func (s *Schema) MakeSelect(result interface{}, filter Filter, options *SelectOptions) (*BaseSelectQuery, error) {
	ptr := reflect.ValueOf(result)
	typ, err := checkQueryTypeShape(ptr.Type())
	if err != nil {
		return nil, err
	}

	return s.makeSelect(typ, filter, options)
}

var errBadQueryRowType = errors.New("query row result should be a pointer to a pointer to a struct")

func checkQueryRowTypeShape(typ reflect.Type) (reflect.Type, error) {
	if typ.Kind() != reflect.Ptr {
		return nil, errBadQueryRowType
	}
	typ = typ.Elem()
	if typ.Kind() != reflect.Ptr {
		return nil, errBadQueryRowType
	}
	typ = typ.Elem()
	if typ.Kind() != reflect.Struct {
		return nil, errBadQueryRowType
	}
	return typ, nil
}

func (s *Schema) MakeSelectRow(result interface{}, filter Filter, options *SelectOptions) (*BaseSelectQuery, error) {
	ptr := reflect.ValueOf(result)
	typ, err := checkQueryRowTypeShape(ptr.Type())
	if err != nil {
		return nil, err
	}

	return s.makeSelect(typ, filter, options)
}

var errBadMutateRowType = errors.New("mutate row value should be a pointer to a struct")

func checkMutateRowTypeShape(typ reflect.Type) (reflect.Type, error) {
	if typ.Kind() != reflect.Ptr {
		return nil, errBadMutateRowType
	}
	typ = typ.Elem()
	if typ.Kind() != reflect.Struct {
		return nil, errBadMutateRowType
	}
	return typ, nil
}

// MakeInsertRow builds a new InsertQuery to insert row
func (s *Schema) MakeInsertRow(row interface{}) (*InsertQuery, error) {
	ptr := reflect.ValueOf(row)
	typ, err := checkMutateRowTypeShape(ptr.Type())
	if err != nil {
		return nil, err
	}
	table, err := s.get(typ)
	if err != nil {
		return nil, err
	}

	allValues, err := table.unbuildStruct(row)
	if err != nil {
		return nil, err
	}
	var columns []string
	var values []interface{}

	for i, column := range table.Columns {
		if column.Primary && table.PrimaryKeyType == AutoIncrement {
			continue
		}
		columns = append(columns, column.Name)
		values = append(values, allValues[i])
	}

	return &InsertQuery{
		Table:   table.Name,
		Columns: columns,
		Values:  values,
	}, nil
}

// MakeUpsertRow builds a new UpsertQuery to upsqrt row
func (s *Schema) MakeUpsertRow(row interface{}) (*UpsertQuery, error) {
	ptr := reflect.ValueOf(row)
	typ, err := checkMutateRowTypeShape(ptr.Type())
	if err != nil {
		return nil, err
	}

	table, err := s.get(typ)
	if err != nil {
		return nil, err
	}

	if table.PrimaryKeyType == AutoIncrement {
		return nil, errors.New("upsert only supports unique value primary keys")
	}

	values, err := table.unbuildStruct(row)
	if err != nil {
		return nil, err
	}
	var columns []string
	for _, column := range table.Columns {
		columns = append(columns, column.Name)
	}

	return &UpsertQuery{
		Table:   table.Name,
		Columns: columns,
		Values:  values,
	}, nil
}

// MakeUpdateRow builds a new UpdateQuery to update row
func (s *Schema) MakeUpdateRow(row interface{}) (*UpdateQuery, error) {
	ptr := reflect.ValueOf(row)
	typ, err := checkMutateRowTypeShape(ptr.Type())
	if err != nil {
		return nil, err
	}

	table, err := s.get(typ)
	if err != nil {
		return nil, err
	}

	allValues, err := table.unbuildStruct(row)
	if err != nil {
		return nil, err
	}
	var columns, whereColumns []string
	var values, whereValues []interface{}

	for i, column := range table.Columns {
		if column.Primary {
			whereColumns = append(whereColumns, column.Name)
			whereValues = append(whereValues, allValues[i])
		} else {
			columns = append(columns, column.Name)
			values = append(values, allValues[i])
		}
	}

	return &UpdateQuery{
		Table:   table.Name,
		Columns: columns,
		Values:  values,
		Where: &SimpleWhere{
			Columns: whereColumns,
			Values:  whereValues,
		},
	}, nil
}

// MakeDeleteRow builds a new DeleteQuery to delete row
func (s *Schema) MakeDeleteRow(row interface{}) (*DeleteQuery, error) {
	ptr := reflect.ValueOf(row)
	typ, err := checkMutateRowTypeShape(ptr.Type())
	if err != nil {
		return nil, err
	}
	table, err := s.get(typ)
	if err != nil {
		return nil, err
	}

	allValues, err := table.unbuildStruct(row)
	if err != nil {
		return nil, err
	}
	var columns []string
	var values []interface{}
	for i, column := range table.Columns {
		if !column.Primary {
			continue
		}

		columns = append(columns, column.Name)
		values = append(values, allValues[i])
	}

	return &DeleteQuery{
		Table: table.Name,
		Where: &SimpleWhere{
			Columns: columns,
			Values:  values,
		},
	}, nil
}

type Tester interface {
	Test(row interface{}) bool
}

// tester tests rows against a filter
type tester struct {
	columns []*Column
	values  []interface{}
}

// coerce coerces some types for more idiomatic comparisons
//
// any nil becomes the interface{} nil so that coerce(*int(nil)) == coerce(interface{}(nil))
// any pointer get dereferenced once so that coerce(&3) == coerce(3)
func coerce(v reflect.Value) interface{} {
	if v == reflect.ValueOf(nil) || (v.Kind() == reflect.Ptr && v.IsNil()) {
		return nil
	}

	if v.Kind() == reflect.Ptr {
		return v.Elem().Interface()
	}

	return v.Interface()
}

func coerceMap(m map[string]interface{}) map[string]interface{} {
	c := make(map[string]interface{})
	for k, v := range m {
		c[k] = coerce(reflect.ValueOf(v))
	}
	return c
}

func (t *tester) Test(row interface{}) bool {
	if row == nil {
		return false
	}

	struc := reflect.ValueOf(row).Elem()
	for i, column := range t.columns {
		expected, err := column.Descriptor.Valuer(reflect.ValueOf(t.values[i])).Value()
		if err != nil {
			// Ignore error.
			return false
		}
		value, err := column.Descriptor.Valuer(struc.FieldByIndex(column.Index)).Value()
		if err != nil {
			// Ignore error.
			return false
		}

		if !driverValuesEqual(expected, value) {
			return false
		}
	}

	return true
}

func (s *Schema) MakeTester(table string, filter Filter) (Tester, error) {
	t, ok := s.ByName[table]
	if !ok {
		return nil, errors.New("unknown table")
	}

	columns := []*Column{}
	values := []interface{}{}

	for name, value := range filter {
		column, ok := t.ColumnsByName[name]
		if !ok {
			return nil, fmt.Errorf("unknown column %s", name)
		}
		columns = append(columns, column)
		values = append(values, value)
	}

	return &tester{
		columns: columns,
		values:  values,
	}, nil
}

func (t *Table) extractRow(row interface{}) Filter {
	f := make(Filter)

	struc := reflect.ValueOf(row).Elem()
	for _, column := range t.Columns {
		f[column.Name] = struc.FieldByIndex(column.Index).Interface()
	}

	return f
}

// driverValuesEqual returns true if two driver.Values are identical.
// driver.Value must be one of the following types
//
//   int64
//   float64
//   bool
//   []byte
//   string
//   time.Time
func driverValuesEqual(dv1, dv2 driver.Value) bool {
	k1 := reflect.ValueOf(dv1).Kind()
	k2 := reflect.ValueOf(dv2).Kind()

	// Kinds must match.
	if k1 != k2 {
		return false
	}

	// Special case: if both driver.Value are slices, compare their byte values.
	if k1 == reflect.Slice || k2 == reflect.Slice {
		if b1, ok := dv1.([]byte); ok {
			if b2, ok := dv2.([]byte); ok {
				return bytes.Compare(b1, b2) == 0
			}
		}
		return false
	}

	// Naive equality check for remaining primitive types.
	if dv1 == dv2 {
		return true
	}

	return false
}

// UnbuildStruct extracts SQL values from a struct.
func (s *Schema) UnbuildStruct(tableName string, strct interface{}) ([]interface{}, error) {
	table, ok := s.ByName[tableName]
	if !ok {
		return nil, fmt.Errorf("unknown table: %s", tableName)
	}
	return table.unbuildStruct(strct)
}

// BuildStruct scans a row in struct's column order into a struct pointer.
func (s *Schema) BuildStruct(tableName string, row []driver.Value) (interface{}, error) {
	table, ok := s.ByName[tableName]
	if !ok {
		return nil, fmt.Errorf("unknown table: %s", tableName)
	}

	ptr := reflect.New(table.Type)
	elem := ptr.Elem()

	if len(row) != len(table.Columns) {
		return nil, fmt.Errorf("row has %d columns but table %s struct %s has %d columns",
			len(row), table.Name, table.Type.String(), len(table.Columns))
	}

	scanners := table.Scanners.Get().([]interface{})
	defer table.Scanners.Put(scanners)

	for i := range row {
		field := elem.FieldByIndex(table.Columns[i].Index)
		if field.Kind() != reflect.Ptr {
			field = field.Addr()
		}
		scanner := scanners[i].(*fields.Scanner)
		scanner.Target(field)
		if err := scanner.Scan(row[i]); err != nil {
			return nil, fmt.Errorf("`%s`.`%s` error: %v", table.Name, table.Columns[i].Name, err)
		}
	}

	return ptr.Interface(), nil
}
