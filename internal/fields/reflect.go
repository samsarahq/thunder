package fields

import (
	"reflect"
)

// isZero returns whether the value of the passed in reflect value is the
// zero value of that type.
// Mostly copied from https://github.com/golang/go/issues/7501
// Works for mostly everything except zero initialized arrays (e.g.
// `var ray [5]string` will not be "Zero")
func isZero(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return v.IsNil()
	case reflect.Struct:
		// This is a special case comparison for struct types.
		return v.Interface() == reflect.Zero(v.Type()).Interface()
	case reflect.Invalid:
		// Invalid types are, by definition, "Zero" values
		return true
	}

	return false
}
