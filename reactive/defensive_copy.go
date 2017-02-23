package reactive

import (
	"reflect"
	"unsafe"
)

// A kv holds a snapshot of a map key-value pair. We snapshot the key as well
// because points to structs are valid keys and the contents of the struct
// could change while the identity of the map key would not change.
type kv struct {
	originalKey reflect.Value
	key, value  *snapshot
}

// A snapshot holds a copy of the contents of a reflect.Value.
type snapshot struct {
	valid bool

	typ reflect.Type

	values []*snapshot // used for array, slice, and struct
	value  *snapshot   // used for interface and ptr
	kvs    []kv        // used for map
	raw    interface{} // used for everything else
}

// extractValue extracts a value out of a reflect.Value. This should return the
// same result as reflect.Value.Interface(), except that it's illegal to call
// Interface() on unexported fields.
func extractValue(v reflect.Value) interface{} {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int()
	case reflect.Uint, reflect.Uintptr, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint()
	case reflect.Float32, reflect.Float64:
		return v.Float()
	case reflect.Complex64, reflect.Complex128:
		return v.Complex()
	case reflect.Bool:
		return v.Bool()
	case reflect.String:
		return v.String()
	case reflect.Chan, reflect.UnsafePointer:
		return v.Pointer()
	default:
		panic("unexpected type " + v.Type().String())
	}
}

// visit is used to track already snapshotted values to handle circular object
// graphs.
type visit struct {
	addr unsafe.Pointer
	typ  reflect.Type
	len  int
	o    *snapshot
}

// deepSnapshot takes a snapshot of a value to be used in place of a deep copy.
// The reflect package has no support for deep copies because such a copy would
// involve manipulating unexported state. This snapshot function instead
// returns a similar-but-different copy of the original values stored in the
// snapshot struct.
//
// deepSnapshot is based on reflect.DeepCopy and can take snapshots of value
// that are comparable using reflect.DeepCopy. Function pointers are not
// supported.
func deepSnapshot(v reflect.Value, visited map[visit]*snapshot) *snapshot {
	if !v.IsValid() {
		return &snapshot{valid: false}
	}

	result := &snapshot{
		valid: true,
		typ:   v.Type(),
	}

	visit := visit{typ: v.Type()}
	memoize := false

	k := v.Kind()
	if v.CanAddr() && (k == reflect.Array || k == reflect.Struct) {
		memoize = true
		visit.addr = unsafe.Pointer(v.UnsafeAddr())
	} else if k == reflect.Slice || k == reflect.Map {
		memoize = true
		visit.addr = unsafe.Pointer(v.Pointer())
		visit.len = v.Len()
	}

	if memoize {
		// Short circuit if we've already seen this value.
		if snapshot, ok := visited[visit]; ok {
			return snapshot
		}

		// Remember for later.
		visited[visit] = result
	}

	switch v.Kind() {
	case reflect.Array:
		result.values = make([]*snapshot, v.Len())
		for i := 0; i < v.Len(); i++ {
			result.values[i] = deepSnapshot(v.Index(i), visited)
		}

	case reflect.Slice:
		if !v.IsNil() {
			result.values = make([]*snapshot, v.Len())
			for i := 0; i < v.Len(); i++ {
				result.values[i] = deepSnapshot(v.Index(i), visited)
			}
		}

	case reflect.Interface, reflect.Ptr:
		if !v.IsNil() {
			result.value = deepSnapshot(v.Elem(), visited)
		}

	case reflect.Struct:
		n := v.NumField()
		result.values = make([]*snapshot, n)
		for i := 0; i < n; i++ {
			result.values[i] = deepSnapshot(v.Field(i), visited)
		}

	case reflect.Map:
		if !v.IsNil() {
			result.kvs = make([]kv, 0, v.Len())
			for _, key := range v.MapKeys() {
				result.kvs = append(result.kvs, kv{
					originalKey: key,
					key:         deepSnapshot(key, visited),
					value:       deepSnapshot(v.MapIndex(key), visited),
				})
			}
		}

	default:
		result.raw = extractValue(v)
	}

	return result
}

func deepCompare(v reflect.Value, o *snapshot, visited map[visit]bool) bool {
	if !v.IsValid() {
		return o.valid == false
	}

	if o.valid != true || o.typ != v.Type() {
		return false
	}

	visit := visit{typ: v.Type(), o: o}
	memoize := false

	k := v.Kind()
	if v.CanAddr() && (k == reflect.Array || k == reflect.Struct) {
		memoize = true
		visit.addr = unsafe.Pointer(v.UnsafeAddr())
	} else if k == reflect.Slice || k == reflect.Map {
		memoize = true
		visit.addr = unsafe.Pointer(v.Pointer())
		visit.len = v.Len()
	}

	if memoize {
		// Short circuit if we've already seen this value.
		if visited[visit] {
			return true
		}

		// Remember for later.
		visited[visit] = true
	}

	switch v.Kind() {
	case reflect.Array:
		if v.Len() != len(o.values) {
			return false
		}
		for i := 0; i < len(o.values); i++ {
			if !deepCompare(v.Index(i), o.values[i], visited) {
				return false
			}
		}

	case reflect.Slice:
		if v.IsNil() != (o.values == nil) {
			return false
		}
		if !v.IsNil() {
			if v.Len() != len(o.values) {
				return false
			}
			for i := 0; i < len(o.values); i++ {
				if !deepCompare(v.Index(i), o.values[i], visited) {
					return false
				}
			}
		}

	case reflect.Interface, reflect.Ptr:
		if v.IsNil() != (o.value == nil) {
			return false
		}
		if !v.IsNil() {
			if !deepCompare(v.Elem(), o.value, visited) {
				return false
			}
		}

	case reflect.Struct:
		n := v.NumField()
		for i := 0; i < n; i++ {
			if !deepCompare(v.Field(i), o.values[i], visited) {
				return false
			}
		}

	case reflect.Map:
		if v.IsNil() != (o.kvs == nil) {
			return false
		}
		if !v.IsNil() {
			if v.Len() != len(o.kvs) {
				return false
			}
			for _, kv := range o.kvs {
				if !deepCompare(kv.originalKey, kv.key, visited) {
					return false
				}
				if !deepCompare(v.MapIndex(kv.originalKey), kv.value, visited) {
					return false
				}
			}
		}

	default:
		if o.raw != extractValue(v) {
			return false
		}
	}

	return true
}

func defensiveCopy(x interface{}) *snapshot {
	return deepSnapshot(reflect.ValueOf(x), make(map[visit]*snapshot))
}

func verifyDefensiveCopy(x interface{}, o *snapshot) bool {
	return deepCompare(reflect.ValueOf(x), o, make(map[visit]bool))
}
