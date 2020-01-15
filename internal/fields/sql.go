package fields

import (
	"database/sql"
	"database/sql/driver"
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/gogo/protobuf/proto"
)

// Valuer fulfills the sql/driver.Valuer interface which deserializes our
// struct field value into a valid SQL value.
type Valuer struct {
	*Descriptor
	value reflect.Value
}

var marshalerType = reflect.TypeOf((*marshaler)(nil)).Elem()

func nonPointerMarshal(d *Descriptor, val reflect.Value) (reflect.Value, bool) {
	if !d.Ptr && reflect.PtrTo(d.Type).Implements(marshalerType) {
		v := reflect.New(d.Type)
		v.Elem().Set(val)
		return v, true
	}
	return val, false
}

var protoMessageType = reflect.TypeOf((*proto.Message)(nil)).Elem()

func nonPointerProtoMessage(d *Descriptor, val reflect.Value) (reflect.Value, bool) {
	if !d.Ptr && reflect.PtrTo(d.Type).Implements(protoMessageType) {
		v := reflect.New(d.Type)
		v.Elem().Set(val)
		return v, true
	}
	return val, false
}

// Value satisfies the sql/driver.Valuer interface.
// The value should be one of the following:
//    int64
//    float64
//    bool
//    []byte
//    string
//    time.Time
//    nil - for NULL values
func (f Valuer) Value() (driver.Value, error) {
	// Return early if the value is nil. Ideally we would do a `i == nil` comparison here, but
	// unfortunately for us, `nil` is typed and that would always return false. This has to be
	// before `.Interface()` as that method panics otherwise.
	switch f.value.Kind() {
	// IsNil panics if the value isn't one of these kinds.
	case reflect.Chan, reflect.Map, reflect.Func,
		reflect.Ptr, reflect.Interface, reflect.Slice:
		if f.value.IsNil() {
			return nil, nil
		}
	case reflect.Invalid:
		return nil, nil
	}

	i := f.value.Interface()

	// If our interface supports driver.Valuer we can immediately short-circuit as this is what the
	// MySQL driver would do.
	if valuer, ok := i.(driver.Valuer); ok {
		return valuer.Value()
	}

	// Override serialization behavior with tags (these take precedence over how a type would
	// usually be serialized).
	// Example:
	// struct {
	//   Blob proto.Blob `sql:",binary"`       // ensures that Marshal or MarshalBinary is used.
	//   IP IP `sql:",string"`                 // ensures that its MarshalText method
	//	                                       // is used for serialization.
	//   JSON map[string]string `sql:",json"`  // ensures that json.Marshal is used on the value.
	// }
	switch {
	case f.Tags.Contains("binary"):
		if v, ok := nonPointerMarshal(f.Descriptor, f.value); ok {
			return v.Interface().(marshaler).Marshal()
		}
		if iface, ok := i.(marshaler); ok {
			return iface.Marshal()
		}
		if iface, ok := i.(encoding.BinaryMarshaler); ok {
			return iface.MarshalBinary()
		}
		if v, ok := nonPointerProtoMessage(f.Descriptor, f.value); ok {
			return proto.Marshal(v.Interface().(proto.Message))
		}
		if iface, ok := i.(proto.Message); ok {
			return proto.Marshal(iface)
		}
	case f.Tags.Contains("string"):
		if iface, ok := i.(encoding.TextMarshaler); ok {
			return iface.MarshalText()
		}
	case f.Tags.Contains("json"):
		if iface, ok := i.(json.Marshaler); ok {
			return iface.MarshalJSON()
		}
		return json.Marshal(i)
	case f.Tags.Contains("implicitnull"):
		if isZero(f.value) {
			return nil, nil
		}
	}

	// At this point we have already handled `nil` above, so we can assume that all
	// other values can be coerced into dereferenced types of bool/int/float/string.
	if f.value.Kind() == reflect.Ptr {
		f.value = f.value.Elem()
	}

	// Coerce our value into a valid sql/driver.Value (see sql/driver.IsValue).
	// This not only converts base types into their sql counterparts (like int32 -> int64) but also
	// handles custom types (like `type customString string` -> string).
	switch f.value.Kind() {
	case reflect.Bool:
		return f.value.Bool(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return f.value.Int(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(f.value.Uint()), nil
	case reflect.Float32, reflect.Float64:
		return f.value.Float(), nil
	case reflect.String:
		return f.value.String(), nil
	}

	// If we can't figure out what the type is supposed to be, we pass it straight through to SQL,
	// which will return an error if it can't handle it.
	// This means we don't have to handle []byte or time.Time specially, since they'll just pass on
	// through.
	return f.value.Interface(), nil
}

var _ driver.Valuer = Valuer{}
var _ driver.Valuer = &Valuer{}

// Scanner fulfills the sql.Scanner interface which deserializes SQL values
// into the type dictated by our descriptor.
type Scanner struct {
	*Descriptor
	value reflect.Value
}

func (s *Scanner) Target(value reflect.Value) {
	s.value = value
}

// Scan satisfies the sql.Scanner interface.
// The src value will be one of the following:
//    int64
//    float64
//    bool
//    []byte
//    string
//    time.Time
//    nil - for NULL values
func (s *Scanner) Scan(src interface{}) error {
	// Clear out the value after a scan so we aren't holding onto references.
	defer func() { s.value = reflect.Value{} }()

	// Keep track of whether our value was empty.
	isValid := src != nil

	if isValid && s.Ptr {
		s.value.Set(reflect.New(s.Type))
	}

	// Get a value of the pointer of our type. The Scanner and Unmarshalers should
	// only be implemented as dereference methods, since they would do nothing otherwise. Therefore
	// we can safely assume that we should check for these interfaces on the pointer value.
	i := s.value.Interface()
	// Our value however should continue referencing a non-pointer for easier assignment.
	s.value = s.value.Elem()

	// If our interface supports sql.Scanner we can immediately short-circuit as this is what the
	// MySQL driver would do.
	if scanner, ok := i.(sql.Scanner); ok {
		// If the scanner base type is nullable (pointer or one of the below), make it nil,
		// otherwise allow it to scan and handle nil.
		if s.Ptr && src == nil {
			return nil
		}
		switch s.Kind {
		case reflect.Chan, reflect.Map, reflect.Func, reflect.Interface, reflect.Slice:
			if src == nil {
				return nil
			}
		}

		// If we have a scanner it will handle its own validity.
		isValid = true
		return scanner.Scan(src)
	}

	// Null values are simply set to zero. Because we're not holding on to pointers, we need to
	// represent this as a boolean. This comes _after_ the scanner step, just in case the scanner
	// handles nil differently.
	if !isValid {
		return nil
	}

	// Handle coercion into native types []byte and time.Time (this method will return an error if
	// we don't handle them). These are pointers here because we want to pass around a pointer
	// for interfaces.
	switch i.(type) {
	case *[]byte:
		if str, ok := src.(string); ok {
			s.value.Set(reflect.ValueOf([]byte(str)))
			return nil
		}
		if b, ok := src.([]byte); ok {
			bCopy := make([]byte, len(b), len(b))
			copy(bCopy, b)
			s.value.Set(reflect.ValueOf(bCopy))
			return nil
		}
	case *time.Time:
		t := mysql.NullTime{}
		if err := t.Scan(src); err != nil {
			return err
		}
		s.value.Set(reflect.ValueOf(t.Time))
		return nil
	}

	// Override deserialization behavior with tags (these take precedence over how a type would
	// usually be deserialized).
	// Example:
	// struct {
	//   Blob proto.Blob `sql:",binary"`       // ensures that Unmarshal or UnmarshalBinary is used.
	//   IP IP `sql:",string"`                 // ensures that its UnmarshalText method
	//	                                       // is used for deserialization.
	//   JSON map[string]string `sql:",json"`  // ensures that json.Unmarshal is used on the value.
	// }
	switch {
	case s.Tags.Contains("binary"):
		b, ok := src.([]byte)
		if !ok {
			return fmt.Errorf("binary column must be of type []byte, got %T", src)
		}
		if iface, ok := i.(unmarshaler); ok {
			return iface.Unmarshal(b)
		}
		if iface, ok := i.(encoding.BinaryUnmarshaler); ok {
			return iface.UnmarshalBinary(b)
		}
		if iface, ok := i.(proto.Message); ok {
			return proto.Unmarshal(b, iface)
		}
	case s.Tags.Contains("string"):
		if str, ok := src.(string); ok {
			src = []byte(str)
		}
		b, isByte := src.([]byte)
		if !isByte {
			return fmt.Errorf("string/text column must be of type []byte or string, got %T", src)
		}
		if iface, ok := i.(encoding.TextUnmarshaler); isByte && ok {
			return iface.UnmarshalText(b)
		}
	case s.Tags.Contains("json"):
		if str, ok := src.(string); ok {
			src = []byte(str)
		}
		b, isByte := src.([]byte)
		if !isByte {
			return fmt.Errorf("json column must be of type string or []byte, got %T", src)
		}
		// Implicitly will check for json.Unmarshaler.
		return json.Unmarshal(b, i)
	}

	// If a MySQL value can be coerced into our type, we do so here.
	// This not only converts base types into their sql counterparts (like int64 -> int32) but also
	// handles custom types (like string -> `type customString string`).
	switch s.Kind {
	case reflect.Bool:
		b := sql.NullBool{}
		if err := b.Scan(src); err != nil {
			return err
		}
		s.value.Set(reflect.ValueOf(b.Bool).Convert(s.Type))
		return nil
	case
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		i := sql.NullInt64{}
		if err := i.Scan(src); err != nil {
			return err
		}
		s.value.Set(reflect.ValueOf(i.Int64).Convert(s.Type))
		return nil
	case reflect.Float32, reflect.Float64:
		float := sql.NullFloat64{}
		if err := float.Scan(src); err != nil {
			return err
		}
		s.value.Set(reflect.ValueOf(float.Float64).Convert(s.Type))
		return nil
	case reflect.String:
		str := sql.NullString{}
		if err := str.Scan(src); err != nil {
			return err
		}
		s.value.Set(reflect.ValueOf(str.String).Convert(s.Type))
		return nil
	}

	return fmt.Errorf("couldn't coerce type %T into %T", src, i)
}

var _ sql.Scanner = &Scanner{}
