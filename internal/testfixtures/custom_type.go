package testfixtures

import (
	"database/sql/driver"
	"fmt"
)

type CustomType [16]byte

func CustomTypeFromString(s string) CustomType {
	b := []byte(s)
	c := CustomType{}
	copy(c[:], b)
	return c
}

func (u CustomType) String() string {
	return string([]byte(u[:]))
}

func (u CustomType) Value() (driver.Value, error) {
	return []byte(u[:]), nil
}

func (u *CustomType) Scan(value interface{}) error {
	switch value := value.(type) {
	case nil:
		u = nil
	case string:
		b := []byte(value)
		copy(u[:], b)
	case []byte:
		copy(u[:], value)
	default:
		return fmt.Errorf("cannot convert %v (of type %T) to %T", value, value, u)
	}
	return nil
}
