package fields_test

import (
	"database/sql/driver"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/samsarahq/thunder/internal/fields"
	"github.com/samsarahq/thunder/internal/proto"
	"github.com/stretchr/testify/assert"
)

type likeBool bool
type likeString string
type likeInt int16
type likeFloat float32

// Fulfills marshaler and unmarshaler interfaces.
type ifaceMarshal struct{ Text string }

func (i ifaceMarshal) Marshal() ([]byte, error) { return []byte(i.Text), nil }
func (i *ifaceMarshal) Unmarshal(b []byte) error {
	i.Text = string(b)
	return nil
}

// Fulfills encoding.BinaryMarshaler and encoding.BinaryUnmarshaler interfaces.
type ifaceBinaryMarshal struct{ Text string }

func (i ifaceBinaryMarshal) MarshalBinary() ([]byte, error) { return []byte(i.Text), nil }
func (i *ifaceBinaryMarshal) UnmarshalBinary(b []byte) error {
	i.Text = string(b)
	return nil
}

// Fulfills encoding.TextMarshaler and encoding.TextUnmarshaler interfaces.
type ifaceTextMarshal struct{ Text string }

func (i ifaceTextMarshal) MarshalText() ([]byte, error) { return []byte(i.Text), nil }
func (i *ifaceTextMarshal) UnmarshalText(b []byte) error {
	i.Text = string(b)
	return nil
}

// Fulfills json.Marshaler and json.Unmarshaler interfaces.
type ifaceJSONMarshal struct{ Text []string }

func (i ifaceJSONMarshal) MarshalJSON() ([]byte, error) { return json.Marshal(i.Text) }
func (i *ifaceJSONMarshal) UnmarshalJSON(b []byte) error {
	return json.Unmarshal(b, &i.Text)
}

type ifaceValuer struct{}

func (ifaceValuer) Value() (driver.Value, error) { return []byte("value"), nil }

type ifaceScanner struct{ In interface{} }

func (i *ifaceScanner) Scan(src interface{}) error {
	i.In = src
	return nil
}

func TestField_Value(t *testing.T) {
	var (
		str = "foo"
		byt = []byte("foo")
		num = int64(200)
		flt = float64(200)
		tru = true
	)

	time := time.Now()
	cases := []struct {
		In    interface{}
		Out   interface{}
		Tag   string
		Error bool
	}{
		// Native types:
		{In: "foo", Out: "foo"},
		{In: &str, Out: str},
		{In: []byte("foo"), Out: []byte("foo")},
		{In: &byt, Out: byt},
		{In: int64(200), Out: int64(200)},
		{In: &num, Out: num},
		{In: float64(200), Out: float64(200)},
		{In: &flt, Out: flt},
		{In: true, Out: true},
		{In: &tru, Out: tru},
		{In: time, Out: time},
		{In: &time, Out: time},
		// Type aliases:
		{In: likeString("foo"), Out: "foo"},
		{In: int8(5), Out: int64(5)},
		{In: int16(5), Out: int64(5)},
		{In: int32(5), Out: int64(5)},
		{In: likeInt(5), Out: int64(5)},
		{In: float32(5), Out: float64(5)},
		{In: likeFloat(5), Out: float64(5)},
		// Interfaces without tags:
		{In: ifaceValuer{}, Out: []byte("value")},
		{In: ifaceMarshal{"binary_one"}, Out: ifaceMarshal{"binary_one"}},
		{In: ifaceBinaryMarshal{"binary_two"}, Out: ifaceBinaryMarshal{"binary_two"}},
		{In: ifaceTextMarshal{"text"}, Out: ifaceTextMarshal{"text"}},
		{In: ifaceJSONMarshal{[]string{"json"}}, Out: ifaceJSONMarshal{[]string{"json"}}},
		// Interfaces with tags:
		{In: ifaceMarshal{"binary_one"}, Out: []byte("binary_one"), Tag: "binary"},
		{In: ifaceBinaryMarshal{"binary_two"}, Out: []byte("binary_two"), Tag: "binary"},
		{In: ifaceTextMarshal{"text"}, Out: []byte("text"), Tag: "string"},
		{In: ifaceJSONMarshal{[]string{"json"}}, Out: []byte("[\"json\"]"), Tag: "json"},
	}

	for _, c := range cases {
		typ := reflect.TypeOf(c.In)
		descriptor := fields.New(typ, []string{c.Tag})
		valuer := descriptor.Valuer(reflect.ValueOf(c.In))

		out, err := valuer.Value()
		if c.Error {
			assert.NotNil(t, err)
		} else {
			assert.NoError(t, err)
			assert.Equal(t, c.Out, out)
		}
	}
}

func TestField_Scan(t *testing.T) {
	timeNow := time.Now()
	timeStr := "2008-02-02 12:12:12.000000"
	timeFromStr, err := time.Parse("2006-01-02 15:04:05.000000", timeStr)
	assert.NoError(t, err)

	cases := []struct {
		Type  interface{}
		In    interface{}
		Out   interface{}
		Tag   string
		Error bool
	}{
		// Native types:
		{Type: "", Out: "foo", In: "foo"},
		{Type: []byte{}, Out: []byte("foo"), In: []byte("foo")},
		{Type: int64(0), Out: int64(200), In: int64(200)},
		{Type: float64(0), Out: float64(200), In: float64(200)},
		{Type: true, Out: true, In: true},
		{Type: timeNow, Out: timeNow, In: timeNow},
		{Type: timeNow, Out: timeFromStr, In: timeStr},
		{Type: timeNow, Out: time.Time{}, In: nil},
		{Type: &timeNow, Out: &timeFromStr, In: timeStr},
		{Type: &timeNow, Out: (*time.Time)(nil), In: nil},
		// Type aliases:
		{Type: likeString(""), Out: likeString("foo"), In: "foo"},
		{Type: int8(5), Out: int8(5), In: int64(5)},
		{Type: int16(5), Out: int16(5), In: int64(5)},
		{Type: int32(5), Out: int32(5), In: int64(5)},
		{Type: likeInt(5), Out: likeInt(5), In: int64(5)},
		{Type: float32(5), Out: float32(5), In: float64(5)},
		{Type: likeFloat(5), Out: likeFloat(5), In: float64(5)},
		// Interfaces without tags:
		{Type: ifaceScanner{[]byte("scan me")}, Out: ifaceScanner{[]byte("scan me")}, In: []byte("scan me")},
		{Type: ifaceMarshal{}, Out: ifaceMarshal{}, In: []byte{}, Error: true},
		{Type: ifaceBinaryMarshal{}, Out: ifaceBinaryMarshal{}, In: []byte{}, Error: true},
		{Type: ifaceTextMarshal{}, Out: ifaceTextMarshal{}, In: []byte{}, Error: true},
		{Type: ifaceJSONMarshal{}, Out: ifaceJSONMarshal{}, In: []byte{}, Error: true},
		// Pointer scanner with nil:
		{Type: &ifaceScanner{}, Out: (*ifaceScanner)(nil), In: nil},
		// Pointer scanner with value:
		{Type: &ifaceScanner{}, Out: &ifaceScanner{[]byte("scan me")}, In: []byte("scan me")},
		// Interfaces with tags:
		{Type: ifaceMarshal{"binary_one"}, Out: ifaceMarshal{"binary_one"}, In: []byte("binary_one"), Tag: "binary"},
		{Type: ifaceBinaryMarshal{"binary_two"}, Out: ifaceBinaryMarshal{"binary_two"}, In: []byte("binary_two"), Tag: "binary"},
		{Type: ifaceTextMarshal{"text"}, Out: ifaceTextMarshal{"text"}, In: []byte("text"), Tag: "string"},
		{Type: ifaceJSONMarshal{[]string{"json"}}, Out: ifaceJSONMarshal{[]string{"json"}}, In: []byte("[\"json\"]"), Tag: "json"},
		// Pointer interfaces with tags with nil:
		{Type: &ifaceMarshal{}, Out: (*ifaceMarshal)(nil), In: nil, Tag: "binary"},
		{Type: &ifaceBinaryMarshal{}, Out: (*ifaceBinaryMarshal)(nil), In: nil, Tag: "binary"},
		{Type: &ifaceTextMarshal{}, Out: (*ifaceTextMarshal)(nil), In: nil, Tag: "string"},
		{Type: &ifaceJSONMarshal{}, Out: (*ifaceJSONMarshal)(nil), In: nil, Tag: "json"},
		// Pointer interfaces with tags with value:
		{Type: &ifaceMarshal{}, Out: &ifaceMarshal{"binary_one"}, In: []byte("binary_one"), Tag: "binary"},
		{Type: &ifaceBinaryMarshal{}, Out: &ifaceBinaryMarshal{"binary_two"}, In: []byte("binary_two"), Tag: "binary"},
		{Type: &ifaceTextMarshal{}, Out: &ifaceTextMarshal{"text"}, In: []byte("text"), Tag: "string"},
		{Type: &ifaceJSONMarshal{}, Out: &ifaceJSONMarshal{[]string{"json"}}, In: []byte("[\"json\"]"), Tag: "json"},
	}

	for i, c := range cases {
		typ := reflect.TypeOf(c.Type)
		field := fields.New(typ, []string{c.Tag})

		// Hack to hang onto address of pointers (via pointer-pointer)
		var out, ptrptr reflect.Value
		scanner := field.Scanner()
		if field.Ptr {
			ptrptr = reflect.New(reflect.PtrTo(field.Type))
			scanner.Target(ptrptr.Elem())
			out = ptrptr
		} else {
			out = reflect.New(field.Type)
			scanner.Target(out)
		}

		err := scanner.Scan(c.In)
		got := out.Elem().Interface()

		if c.Error {
			assert.NotNil(t, err, "case %d failed", i)
		} else {
			assert.NoError(t, err, "case %d failed", i)
			assert.Equal(t, c.Out, got, "case %d failed", i)
		}
	}
}

func TestField_ValidateSQLType(t *testing.T) {
	integer := int64(0)
	cases := []struct {
		In    interface{}
		Error bool
	}{
		{In: "foo", Error: false},
		{In: int16(0), Error: false},
		{In: &integer, Error: false},
		{In: [4]byte{}, Error: true},
		{In: ifaceBinaryMarshal{}, Error: true},
	}

	for _, c := range cases {
		descriptor := fields.New(reflect.TypeOf(c.In), nil)
		err := descriptor.ValidateSQLType()
		if c.Error {
			assert.NotNil(t, err)
		} else {
			assert.NoError(t, err)
		}
	}
}

func TestField_SupportsProtobuf(t *testing.T) {
	event := &proto.ExampleEvent{Table: "users"}
	descriptor := fields.New(reflect.TypeOf(event), []string{"binary"})
	valuer := descriptor.Valuer(reflect.ValueOf(event))
	b, err := valuer.Value()
	assert.NoError(t, err)

	got := reflect.New(reflect.TypeOf(event)).Elem()
	scanner := descriptor.Scanner()
	scanner.Target(got)
	scanner.Scan(b)
	assert.Equal(t, event, got.Interface())
}
