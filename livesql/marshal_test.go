package livesql

import (
	"testing"

	"github.com/samsarahq/thunder/internal/testfixtures"
	"github.com/samsarahq/thunder/sqlgen"
	"github.com/stretchr/testify/assert"
)

func TestMarshal(t *testing.T) {
	type user struct {
		Id   int64 `sql:",primary"`
		Name *string
		Uuid testfixtures.CustomType
		Mood *testfixtures.CustomType
	}

	schema := sqlgen.NewSchema()
	schema.MustRegisterType("users", sqlgen.AutoIncrement, user{})

	one := int64(1)
	foo := "foo"

	cases := []struct {
		name        string
		filter      sqlgen.Filter
		unmarshaled sqlgen.Filter
		err         bool
	}{
		{
			name:        "nil",
			filter:      nil,
			unmarshaled: sqlgen.Filter{},
		},
		{
			name:        "empty",
			filter:      sqlgen.Filter{},
			unmarshaled: sqlgen.Filter{},
		},
		{
			name:        "uuid",
			filter:      sqlgen.Filter{"uuid": testfixtures.CustomTypeFromString("foo")},
			unmarshaled: sqlgen.Filter{"uuid": testfixtures.CustomTypeFromString("foo")},
		},
		{
			name:        "uuid from bytes",
			filter:      sqlgen.Filter{"uuid": []byte("foo")},
			unmarshaled: sqlgen.Filter{"uuid": testfixtures.CustomTypeFromString("foo")},
		},
		{
			name:        "nil uuid",
			filter:      sqlgen.Filter{"mood": nil},
			unmarshaled: sqlgen.Filter{"mood": (*testfixtures.CustomType)(nil)},
		},
		{
			name:        "id",
			filter:      sqlgen.Filter{"id": int64(1)},
			unmarshaled: sqlgen.Filter{"id": int64(1)},
		},
		{
			name:        "id int32 to int64",
			filter:      sqlgen.Filter{"id": int32(1)},
			unmarshaled: sqlgen.Filter{"id": int64(1)},
		},
		{
			name:        "id int64 ptr to int64",
			filter:      sqlgen.Filter{"id": &one},
			unmarshaled: sqlgen.Filter{"id": int64(1)},
		},
		{
			name:        "string to string ptr",
			filter:      sqlgen.Filter{"name": "foo"},
			unmarshaled: sqlgen.Filter{"name": &foo}},
		{
			name:        "nil to string ptr",
			filter:      sqlgen.Filter{"name": nil},
			unmarshaled: sqlgen.Filter{"name": (*string)(nil)},
		},
		{
			name:   "nil for int64",
			filter: sqlgen.Filter{"id": nil},
			err:    true,
		},
		{
			name:   "string for int64",
			filter: sqlgen.Filter{"id": ""},
			err:    true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			proto, err := filterToProto(schema, "users", c.filter)
			assert.NoError(t, err)

			table, filter, err := filterFromProto(schema, proto)
			if c.err {
				assert.NotNil(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, "users", table)
				assert.Equal(t, c.unmarshaled, filter)
			}
		})
	}
}
