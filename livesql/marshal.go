package livesql

import (
	"database/sql/driver"
	"fmt"
	"reflect"
	"time"

	"github.com/samsarahq/thunder/sqlgen"
	"github.com/samsarahq/thunder/thunderpb"
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

func filterToProto(table string, filter sqlgen.Filter) (*thunderpb.SQLFilter, error) {
	fields := make(map[string]*thunderpb.Field, len(filter))
	for col, val := range filter {
		field, err := valueToField(val)
		if err != nil {
			return nil, err
		}
		fields[col] = field
	}
	return &thunderpb.SQLFilter{Table: table, Fields: fields}, nil
}

func filterFromProto(proto *thunderpb.SQLFilter) (string, sqlgen.Filter, error) {
	filter := make(sqlgen.Filter, len(proto.Fields))
	for col, field := range proto.Fields {
		val, err := fieldToValue(field)
		if err != nil {
			return "", nil, err
		}
		filter[col] = val
	}
	return proto.Table, filter, nil
}
