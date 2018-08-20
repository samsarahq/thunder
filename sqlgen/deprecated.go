package sqlgen

import (
	"database/sql/driver"
	"fmt"
)

type NullBytes struct {
	Bytes []byte
	Valid bool
}

func (b *NullBytes) Scan(value interface{}) error {
	if value == nil {
		b.Bytes = nil
		b.Valid = false
	}
	switch value := value.(type) {
	case nil:
		b.Bytes = nil
		b.Valid = false
	case []byte:
		// copy value since the MySQL driver reuses buffers
		b.Bytes = make([]byte, len(value))
		copy(b.Bytes, value)
		b.Valid = true
	case string:
		b.Bytes = []byte(value)
		b.Valid = true
	default:
		return fmt.Errorf("cannot convert %v to bytes", value)
	}
	return nil
}

func (b *NullBytes) Value() (driver.Value, error) {
	if !b.Valid {
		return nil, nil
	}
	return b.Bytes, nil
}
