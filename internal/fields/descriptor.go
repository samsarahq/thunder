package fields

import (
	"database/sql/driver"
	"fmt"
	"reflect"
)

// Descriptor is a cache object that holds onto relevant information about our struct field and
// allows us not to worry about dealing with pointers during the coercion process.
type Descriptor struct {
	Tags TagSet
	Type reflect.Type
	Kind reflect.Kind
	Ptr  bool
}

// New creates a new FieldDescriptor from a type and tags.
func New(t reflect.Type, tags []string) *Descriptor {
	it := &Descriptor{Tags: newTagSet(tags...), Type: t}

	// If the type is a pointer, dereference the type on iType and continue
	// analysis with dereference.
	it.Ptr = t.Kind() == reflect.Ptr
	if it.Ptr {
		it.Type = it.Type.Elem()
	}
	it.Kind = it.Type.Kind()

	return it
}

// Valuer creates a sql/driver.Valuer from the type and value.
func (d *Descriptor) Valuer(val reflect.Value) Valuer {
	// Ideally we would de-reference pointers here in order to simplify how we work with the value.
	// However, some interfaces (I'm looking at you, gogo/protobuf) implement their methods as
	// pointer methods.
	return Valuer{Descriptor: d, value: val}
}

// Scanner creates a sql.Scanner from the descriptor.
func (d *Descriptor) Scanner() *Scanner {
	return &Scanner{Descriptor: d}
}

// ValidateSQLType checks to see if the field is a valid SQL value.
func (d *Descriptor) ValidateSQLType() error {
	var val reflect.Value
	if d.Ptr {
		val = reflect.New(d.Type)
	} else {
		val = reflect.Zero(d.Type)
	}

	valuer := d.Valuer(val)
	sqlVal, err := valuer.Value()
	if err != nil {
		return err
	}
	if ok := driver.IsValue(sqlVal); !ok {
		return fmt.Errorf("%T is not a valid SQL type", sqlVal)
	}

	// We need to hold onto this pointer-pointer in order to make the value addressable.
	var value, ptrptr reflect.Value
	if d.Ptr {
		ptrptr = reflect.New(reflect.PtrTo(d.Type))
		value = ptrptr.Elem()
	} else {
		value = reflect.New(d.Type)
	}

	scanner := d.Scanner()
	scanner.Target(value)
	return scanner.Scan(sqlVal)
}
