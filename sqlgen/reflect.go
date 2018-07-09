package sqlgen

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"unicode"

	"github.com/gogo/protobuf/proto"
	"github.com/samsarahq/thunder/internal"
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

// BuildStruct constructs a struct value defined by table and based on scannables
func BuildStruct(table *Table, values []interface{}) (interface{}, error) {
	ptr := reflect.New(table.Type)
	elem := ptr.Elem()

	for i, column := range table.Columns {
		if err := column.TypeConverter.Unmarshal(values[i], elem.FieldByIndex(column.Index).Addr().Interface()); err != nil {
			return nil, err
		}
	}

	return ptr.Interface(), nil
}

// UnbuildStruct extracts SQL values from a struct
func UnbuildStruct(table *Table, obj interface{}) ([]interface{}, error) {
	elem := reflect.ValueOf(obj)

	values := make([]interface{}, len(table.Columns))
	for i, column := range table.Columns {
		value, err := column.TypeConverter.Marshal(elem.FieldByIndex(column.Index).Interface())
		if err != nil {
			return nil, err
		}
		values[i] = value
	}

	return values, nil
}

// parseQueryRow parses a row from a sql.DB query into a struct
func parseQueryRow(table *Table, scanner *sql.Rows) (interface{}, error) {
	values := make([]interface{}, len(table.Columns))
	for i := range values {
		values[i] = &values[i]
	}

	if err := scanner.Scan(values...); err != nil {
		return nil, err
	}

	return BuildStruct(table, values)
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

	Index []int
	Order int

	TypeConverter TypeConverter
	Type          reflect.Type
}

type Table struct {
	Name           string
	Type           reflect.Type
	PrimaryKeyType PrimaryKeyType

	Columns       []*Column
	ColumnsByName map[string]*Column
}

var protoMessageType reflect.Type

func init() {
	var protoMessage proto.Message
	protoMessageType = reflect.TypeOf(&protoMessage).Elem()
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
				if tag != "primary" || primary {
					return nil, fmt.Errorf("bad type %s: column %s has unexpected tag %s", typ, column, tag)
				}
				primary = true
			}
		}

		if _, ok := columnsByName[column]; ok {
			return nil, fmt.Errorf("bad type %s: duplicate column %s", typ, column)
		}

		var typeConverter TypeConverter
		switch {
		case field.Type.Implements(protoMessageType):
			typeConverter = ProtoConverter{}
		case field.Type.Kind() == reflect.Ptr:
			typeConverter, _ = s.scalarTypes[field.Type.Elem()]
		default:
			typeConverter, _ = s.scalarTypes[field.Type]
		}

		if typeConverter == nil {
			return nil, fmt.Errorf("bad type %s: field %s has unsupported type %s", typ, field.Name, field.Type)
		}

		descriptor := &Column{
			Name:    column,
			Primary: primary,

			Index: field.Index,
			Order: len(columns),

			TypeConverter: typeConverter,
			Type:          field.Type,
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

	return &Table{
		Name:           table,
		Type:           typ,
		PrimaryKeyType: primaryKeyType,

		Columns:       columns,
		ColumnsByName: columnsByName,
	}, nil
}

type Schema struct {
	ByName map[string]*Table
	ByType map[reflect.Type]*Table

	scalarTypes map[reflect.Type]TypeConverter
}

func NewSchema() *Schema {
	scalarTypes := make(map[reflect.Type]TypeConverter)
	for typ, typeConverter := range defaultTypeConverters {
		scalarTypes[typ] = typeConverter
	}

	return &Schema{
		ByName: make(map[string]*Table),
		ByType: make(map[reflect.Type]*Table),

		scalarTypes: scalarTypes,
	}
}

func (s *Schema) RegisterCustomScalar(scalar interface{}, typeConverter TypeConverter) error {
	scalarTyp := reflect.TypeOf(scalar)
	if scalarTyp.Kind() == reflect.Ptr {
		return fmt.Errorf("scalar type %v must not be a pointer", scalarTyp)
	}
	if _, ok := s.scalarTypes[scalarTyp]; ok {
		return fmt.Errorf("duplicate scalar type %v", scalarTyp)
	}
	s.scalarTypes[scalarTyp] = typeConverter
	return nil
}

func (s *Schema) MustRegisterCustomScalar(scalar interface{}, typeConverter TypeConverter) {
	if err := s.RegisterCustomScalar(scalar, typeConverter); err != nil {
		panic(err)
	}
}

func (s *Schema) RegisterSimpleScalar(scalar interface{}) error {
	typ := reflect.TypeOf(scalar)
	for match, scannable := range defaultTypeConverters {
		if internal.TypesIdenticalOrScalarAliases(typ, match) {
			return s.RegisterCustomScalar(scalar, scannable)
		}
	}
	return errors.New("unknown scalar")
}

func (s *Schema) MustRegisterSimpleScalar(scalar interface{}) {
	if err := s.RegisterSimpleScalar(scalar); err != nil {
		panic(err)
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

		l = append(l, whereElem{column: column, value: value})
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

	allValues, err := UnbuildStruct(table, row)
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

	values, err := UnbuildStruct(table, row)
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

	allValues, err := UnbuildStruct(table, row)
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

	allValues, err := UnbuildStruct(table, row)
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
		// coerces some pointer types to make filters more idiomatic
		expected := coerce(reflect.ValueOf(t.values[i]))
		value := coerce(struc.FieldByIndex(column.Index))
		if expected != value {
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
