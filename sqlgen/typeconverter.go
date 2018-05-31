package sqlgen

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/gogo/protobuf/proto"
)

type TypeConverter interface {
	Marshal(goValue interface{}) (sqlValue interface{}, err error)
	Unmarshal(sqlValue, goValue interface{}) error
}

type ScannableValuer interface {
	sql.Scanner
	driver.Valuer
}

type MakeScannableValuer func() ScannableValuer

type ScannableValuerTypeConverter struct {
	MakeScannableValuer
}

func (tc *ScannableValuerTypeConverter) Marshal(goValue interface{}) (sqlValue interface{}, err error) {
	return coerce(reflect.ValueOf(goValue)), nil
}

func (tc *ScannableValuerTypeConverter) Unmarshal(sqlValue, goValue interface{}) error {
	sv := tc.MakeScannableValuer()
	if err := sv.Scan(sqlValue); err != nil {
		return err
	}

	value, err := sv.Value()
	if err != nil {
		return err
	}
	if value == nil {
		return nil
	}

	reflValue := reflect.ValueOf(goValue).Elem()

	if reflValue.Type().Kind() == reflect.Ptr {
		reflValue.Set(reflect.New(reflValue.Type().Elem()))
		reflValue = reflValue.Elem()
	}

	reflValue.Set(reflect.ValueOf(value).Convert(reflValue.Type()))
	return nil
}

type NullBytes struct {
	Bytes []byte
	Valid bool
}

func (b *NullBytes) Scan(value interface{}) error {
	if value == nil {
		b.Bytes = nil
		b.Valid = false
	}
	switch value := value.(type) {
	case nil:
		b.Bytes = nil
		b.Valid = false
	case []byte:
		// copy value since the MySQL driver reuses buffers
		b.Bytes = make([]byte, len(value))
		copy(b.Bytes, value)
		b.Valid = true
	case string:
		b.Bytes = []byte(value)
		b.Valid = true
	default:
		return fmt.Errorf("cannot convert %v to bytes", value)
	}
	return nil
}

func (b *NullBytes) Value() (driver.Value, error) {
	if !b.Valid {
		return nil, nil
	}
	return b.Bytes, nil
}

// Types should implement both the sql.Scanner and driver.Valuer interface.
var defaultTypeConverters = map[reflect.Type]TypeConverter{
	// These types should not be pointer types; pointer types are handled
	// automatically and are treated as optional fields.
	reflect.TypeOf(string("")):  &ScannableValuerTypeConverter{MakeScannableValuer: func() ScannableValuer { return new(sql.NullString) }},
	reflect.TypeOf(int64(0)):    &ScannableValuerTypeConverter{MakeScannableValuer: func() ScannableValuer { return new(sql.NullInt64) }},
	reflect.TypeOf(int32(0)):    &ScannableValuerTypeConverter{MakeScannableValuer: func() ScannableValuer { return new(sql.NullInt64) }},
	reflect.TypeOf(int16(0)):    &ScannableValuerTypeConverter{MakeScannableValuer: func() ScannableValuer { return new(sql.NullInt64) }},
	reflect.TypeOf(bool(false)): &ScannableValuerTypeConverter{MakeScannableValuer: func() ScannableValuer { return new(sql.NullBool) }},
	reflect.TypeOf(float64(0)):  &ScannableValuerTypeConverter{MakeScannableValuer: func() ScannableValuer { return new(sql.NullFloat64) }},
	reflect.TypeOf([]byte{}):    &ScannableValuerTypeConverter{MakeScannableValuer: func() ScannableValuer { return new(NullBytes) }},
	reflect.TypeOf(time.Time{}): &ScannableValuerTypeConverter{MakeScannableValuer: func() ScannableValuer { return new(mysql.NullTime) }},
}

type ProtoConverter struct{}

func (ProtoConverter) Marshal(goValue interface{}) (sqlValue interface{}, err error) {
	bytes, err := proto.Marshal(goValue.(proto.Message))
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func (ProtoConverter) Unmarshal(sqlValue, goValue interface{}) error {
	reflValue := reflect.ValueOf(goValue).Elem()
	reflValue.Set(reflect.New(reflValue.Type().Elem()))
	protoMessage := reflValue.Interface().(proto.Message)
	return proto.Unmarshal(sqlValue.([]byte), protoMessage)
}
