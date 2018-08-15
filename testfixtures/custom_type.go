package testfixtures

import (
	"database/sql/driver"
	"fmt"
)

type CustomType []byte

func CustomTypeFromString(s string) CustomType {
	return CustomType(s)
}

func (u CustomType) Value() (driver.Value, error) {
	return []byte(u), nil
}

func (u *CustomType) Scan(value interface{}) error {
	switch value := value.(type) {
	case nil:
		u = nil
	case []byte:
		*u = make(CustomType, len(value))
		copy(*u, value)
	case string:
		*u = CustomType(value)
	default:
		return fmt.Errorf("cannot convert %v (of type %T) to %T", value, value, u)
	}
	return nil
}
