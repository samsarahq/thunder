package internal

import "reflect"

func IsScalarType(t reflect.Type) bool {
	switch t.Kind() {
	case
		reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Uintptr,
		reflect.Float32, reflect.Float64,
		reflect.Complex64, reflect.Complex128,
		reflect.String:
		return true

	default:
		return false
	}
}

func TypesIdenticalOrScalarAliases(a, b reflect.Type) bool {
	return a == b || (a.Kind() == b.Kind() && IsScalarType(a))
}

// interfaceTyp is the reflect.Type of interface{}
var interfaceTyp reflect.Type

func init() {
	var x interface{}
	interfaceTyp = reflect.TypeOf(&x).Elem()
}

// ToArray converts a []interface{} slice into an equivalent fixed-length array
// [...]interface{} for use as a comparable map key
//
func ToArray(s []interface{}) interface{} {
	switch len(s) {
	// fast code paths for short arrays:
	case 0:
		return [...]interface{}{}
	case 1:
		return [...]interface{}{s[0]}
	case 2:
		return [...]interface{}{s[0], s[1]}
	case 3:
		return [...]interface{}{s[0], s[1], s[2]}
	case 4:
		return [...]interface{}{s[0], s[1], s[2], s[3]}
	case 5:
		return [...]interface{}{s[0], s[1], s[2], s[3], s[4]}
	case 6:
		return [...]interface{}{s[0], s[1], s[2], s[3], s[4], s[5]}
	case 7:
		return [...]interface{}{s[0], s[1], s[2], s[3], s[4], s[5], s[6]}
	case 8:
		return [...]interface{}{s[0], s[1], s[2], s[3], s[4], s[5], s[6], s[7]}
	default:
		// slow catch-all:
		array := reflect.New(reflect.ArrayOf(len(s), interfaceTyp)).Elem()
		for i, elem := range s {
			array.Index(i).Set(reflect.ValueOf(elem))
		}
		return array.Interface()
	}
}
