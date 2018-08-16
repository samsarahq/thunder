package fields_test

import (
	"reflect"
	"testing"

	"github.com/samsarahq/thunder/fields"
	"github.com/stretchr/testify/assert"
)

type customString string

func TestNew(t *testing.T) {
	str := "foo"
	cstr := customString("foo")
	cases := []struct {
		In   interface{}
		Type reflect.Type
		Kind reflect.Kind
		Ptr  bool
	}{
		{In: str, Type: reflect.TypeOf(str), Kind: reflect.String, Ptr: false},
		{In: &str, Type: reflect.TypeOf(str), Kind: reflect.String, Ptr: true},
		{In: cstr, Type: reflect.TypeOf(cstr), Kind: reflect.String, Ptr: false},
		{In: &cstr, Type: reflect.TypeOf(cstr), Kind: reflect.String, Ptr: true},
	}

	for _, c := range cases {
		f := fields.New(reflect.TypeOf(c.In), nil)
		assert.Equal(t, c.Type, f.Type)
		assert.Equal(t, c.Kind, f.Kind)
		assert.Equal(t, c.Ptr, f.Ptr)
	}
}
