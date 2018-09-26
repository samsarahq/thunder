package livesql

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/obad2015/thunder/internal/fields"
	"github.com/obad2015/thunder/sqlgen"
	"github.com/obad2015/thunder/thunderpb"
)

// valueToField converts a driver.Value into a thunderpb.Field.
// driver.Value must be one of the following:
//   int64
//   float64
//   bool
//   []byte
//   string
//   time.Time
func valueToField(value driver.Value) (*thunderpb.Field, error) {
	switch column := value.(type) {
	case nil:
		return &thunderpb.Field{Kind: thunderpb.FieldKind_Null}, nil
	case int64:
		return &thunderpb.Field{Kind: thunderpb.FieldKind_Int, Int: column}, nil
	case float64:
		return &thunderpb.Field{Kind: thunderpb.FieldKind_Float64, Float64: column}, nil
	case bool:
		return &thunderpb.Field{Kind: thunderpb.FieldKind_Bool, Bool: column}, nil
	case []byte:
		return &thunderpb.Field{Kind: thunderpb.FieldKind_Bytes, Bytes: column}, nil
	case string:
		return &thunderpb.Field{Kind: thunderpb.FieldKind_String, String_: column}, nil
	case time.Time:
		return &thunderpb.Field{Kind: thunderpb.FieldKind_Time, Time: column}, nil
	default:
		return nil, fmt.Errorf("unknown type %s", reflect.TypeOf(column))
	}
}

func fieldToValue(field *thunderpb.Field) (driver.Value, error) {
	switch field.Kind {
	case thunderpb.FieldKind_Null:
		return nil, nil
	case thunderpb.FieldKind_Bool:
		return field.Bool, nil
	case thunderpb.FieldKind_Int:
		return field.Int, nil
	case thunderpb.FieldKind_Uint:
		return field.Uint, nil
	case thunderpb.FieldKind_String:
		return field.String_, nil // field.String is a function.
	case thunderpb.FieldKind_Bytes:
		return field.Bytes, nil
	case thunderpb.FieldKind_Float64:
		return field.Float64, nil
	case thunderpb.FieldKind_Time:
		return field.Time, nil
	default:
		return nil, fmt.Errorf("unknown kind %s", field.Kind.String())
	}
}

// filterToProto takes a sqlgen.Filter, runs Valuer on each filter value, and returns a thunderpb.SQLFilter.
func filterToProto(schema *sqlgen.Schema, tableName string, filter sqlgen.Filter) (*thunderpb.SQLFilter, error) {
	table, ok := schema.ByName[tableName]
	if !ok {
		return nil, fmt.Errorf("unknown table: %s", tableName)
	}

	if filter == nil {
		return &thunderpb.SQLFilter{Table: tableName}, nil
	}

	fields := make(map[string]*thunderpb.Field, len(filter))
	for col, val := range filter {
		column, ok := table.ColumnsByName[col]
		if !ok {
			return nil, fmt.Errorf("unknown column %s", col)
		}

		val, err := column.Descriptor.Valuer(reflect.ValueOf(val)).Value()
		if err != nil {
			return nil, err
		}

		field, err := valueToField(val)
		if err != nil {
			return nil, err
		}
		fields[col] = field
	}
	return &thunderpb.SQLFilter{Table: tableName, Fields: fields}, nil
}

// filterFromProto takes a thunderpb.SQLFilter, runs Scanner on each field value, and returns a sqlgen.Filter.
func filterFromProto(schema *sqlgen.Schema, proto *thunderpb.SQLFilter) (string, sqlgen.Filter, error) {
	table, ok := schema.ByName[proto.Table]
	if !ok {
		return "", nil, fmt.Errorf("unknown table: %s", proto.Table)
	}

	scanners := table.Scanners.Get().([]interface{})
	defer table.Scanners.Put(scanners)

	filter := make(sqlgen.Filter, len(proto.Fields))
	for col, field := range proto.Fields {
		val, err := fieldToValue(field)
		if err != nil {
			return "", nil, err
		}

		column, ok := table.ColumnsByName[col]
		if !ok {
			return "", nil, fmt.Errorf("unknown column %s", col)
		}

		if !column.Descriptor.Ptr && val == nil {
			return "", nil, errors.New("cannot unmarshal nil into non-pointer type")
		}

		scanner := scanners[column.Order].(*fields.Scanner)

		// target is always a pointer.
		var target, ptrptr reflect.Value
		if column.Descriptor.Ptr {
			// We need to hold onto this pointer-pointer in order to make the value addressable.
			ptrptr = reflect.New(reflect.PtrTo(column.Descriptor.Type))
			target = ptrptr.Elem()
		} else {
			target = reflect.New(column.Descriptor.Type)
		}
		scanner.Target(target)

		if err := scanner.Scan(val); err != nil {
			return "", nil, err
		}

		if column.Descriptor.Ptr {
			filter[col] = target.Interface()
		} else {
			// Dereference pointer if column type is not a pointer.
			filter[col] = target.Elem().Interface()
		}
	}
	return proto.Table, filter, nil
}
