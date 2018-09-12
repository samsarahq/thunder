package fields_test

import (
	"database/sql/driver"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/samsarahq/thunder/fields"
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
	time := time.Now()
	cases := []struct {
		In    interface{}
		Out   interface{}
		Tag   string
		Error bool
	}{
		// Native types:
		{Out: "foo", In: "foo"},
		{Out: []byte("foo"), In: []byte("foo")},
		{Out: int64(200), In: int64(200)},
		{Out: float64(200), In: float64(200)},
		{Out: true, In: true},
		{Out: time, In: time},
		// Type aliases:
		{Out: likeString("foo"), In: "foo"},
		{Out: int8(5), In: int64(5)},
		{Out: int16(5), In: int64(5)},
		{Out: int32(5), In: int64(5)},
		{Out: likeInt(5), In: int64(5)},
		{Out: float32(5), In: float64(5)},
		{Out: likeFloat(5), In: float64(5)},
		// Interfaces without tags:
		{Out: ifaceScanner{[]byte("scan me")}, In: []byte("scan me")},
		{Out: ifaceValuer{}, In: []byte("value"), Error: true},
		{Out: ifaceMarshal{}, In: ifaceMarshal{}, Error: true},
		{Out: ifaceBinaryMarshal{}, In: ifaceBinaryMarshal{}, Error: true},
		{Out: ifaceTextMarshal{}, In: ifaceTextMarshal{}, Error: true},
		{Out: ifaceJSONMarshal{}, In: ifaceJSONMarshal{}, Error: true},
		// Interfaces with tags:
		{Out: ifaceMarshal{"binary_one"}, In: []byte("binary_one"), Tag: "binary"},
		{Out: ifaceBinaryMarshal{"binary_two"}, In: []byte("binary_two"), Tag: "binary"},
		{Out: ifaceTextMarshal{"text"}, In: []byte("text"), Tag: "string"},
		{Out: ifaceJSONMarshal{[]string{"json"}}, In: []byte("[\"json\"]"), Tag: "json"},
	}

	for i, c := range cases {
		typ := reflect.TypeOf(c.Out)
		field := fields.New(typ, []string{c.Tag})
		scanner := field.Scanner()

		err := scanner.Scan(c.In)
		out := scanner.Interface()

		if c.Error {
			assert.NotNil(t, err, "case %d failed", i)
		} else {
			assert.NoError(t, err, "case %d failed", i)
			assert.Equal(t, c.Out, out, "case %d failed", i)
		}
	}
}

func TestField_ValidateSQLType(t *testing.T) {
	cases := []struct {
		In    interface{}
		Error bool
	}{
		{In: "foo", Error: false},
		{In: int16(0), Error: false},
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

	scanner := descriptor.Scanner()
	scanner.Scan(b)
	assert.Equal(t, *event, scanner.Interface())
}
