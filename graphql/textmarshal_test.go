package graphql_test

import (
	"errors"
	"testing"

	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/samsarahq/thunder/internal/testgraphql"
	"github.com/satori/go.uuid"
)

func TestTextMarshaling(t *testing.T) {
	schema := schemabuilder.NewSchema()

	type Inner struct {
		PtrUuid      *Uuid
		Uuid         Uuid
		UuidSlice    []Uuid
		PtrUuidSlice []*Uuid
	}

	o := schema.Object("Inner", Inner{})
	o.FieldFunc("uuidFunc", func(inner Inner) Uuid {
		return inner.Uuid
	})
	o.FieldFunc("uuidSliceFunc", func(inner Inner) []Uuid {
		return inner.UuidSlice
	})
	o.FieldFunc("ptrUuidSliceFunc", func(inner Inner) []*Uuid {
		return inner.PtrUuidSlice
	})
	o.FieldFunc("invalidUuidFunc", func(inner Inner) Uuid {
		return Uuid{marshalError: errors.New("invalidUUID")} // Invalid Uuid type (for testing)
	})

	PtrUuidSliceResp := []*Uuid{
		NewUuidPtr(),
		nil,
		NewUuidPtr(),
	}

	query := schema.Query()
	query.FieldFunc("inner", func(input struct {
		InputPtrUuid   *Uuid
		InputUuid      Uuid
		InputUuidSlice []Uuid
	}) Inner {
		return Inner{
			PtrUuid:      input.InputPtrUuid,
			Uuid:         input.InputUuid,
			UuidSlice:    input.InputUuidSlice,
			PtrUuidSlice: PtrUuidSliceResp,
		}
	})

	_ = schema.Mutation()

	builtSchema := schema.MustBuild()

	snap := testgraphql.NewSnapshotter(t, builtSchema)
	defer snap.Verify()

	snap.SnapshotQuery("happy path all inputs and outputs", `{
		inner(
			inputPtrUuid: "74771078-5edb-4733-88f2-000000000000", 
			inputUuid: "74771078-5edb-4733-88f2-111111111111", 
			inputUuidSlice: ["74771078-5edb-4733-88f2-222222222222", "74771078-5edb-4733-88f2-333333333333", "74771078-5edb-4733-88f2-444444444444"], 
		) { 
			ptrUuid
			uuid
			uuidSlice
			ptrUuidSlice
			uuidFunc
			uuidSliceFunc
			ptrUuidSliceFunc
		}
	}`)

	snap.SnapshotQuery("invalid ptr uuid input", `{
		inner(
			inputPtrUuid: "invaliduuid", 
			inputUuid: "74771078-5edb-4733-88f2-111111111111", 
			inputUuidSlice: ["74771078-5edb-4733-88f2-222222222222"], 
		) { 
			ptrUuid
		}
	}`, testgraphql.RecordError)

	snap.SnapshotQuery("invalid uuid input", `{
		inner(
			inputPtrUuid: "74771078-5edb-4733-88f2-000000000000", 
			inputUuid: "invaliduuid", 
			inputUuidSlice: ["74771078-5edb-4733-88f2-222222222222"], 
		) { 
			uuid
		}
	}`, testgraphql.RecordError)

	snap.SnapshotQuery("invalid uuid slice input", `{
		inner(
			inputPtrUuid: "74771078-5edb-4733-88f2-000000000000", 
			inputUuid: "74771078-5edb-4733-88f2-111111111111", 
			inputUuidSlice: ["74771078-5edb-4733-88f2-222222222222", "invaliduuid"], 
		) { 
			uuidSlice
		}
	}`, testgraphql.RecordError)

	snap.SnapshotQuery("invalid uuid func response", `{
		inner(
			inputPtrUuid: "74771078-5edb-4733-88f2-000000000000", 
			inputUuid: "74771078-5edb-4733-88f2-111111111111", 
			inputUuidSlice: ["74771078-5edb-4733-88f2-222222222222"], 
		) { 
			invalidUuidFunc
		}
	}`, testgraphql.RecordError)

}

// Uuid is a testable version of a "Text Marshalable" type.
type Uuid struct {
	bytes        [16]byte
	marshalError error
}

func NewUuidPtr() *Uuid {
	u := NewUuid()
	return &u
}

var counter = byte(0)

func NewUuid() Uuid {
	u := Uuid{
		bytes: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, byte(counter)},
	}
	counter += 1
	return u
}

func (u Uuid) MarshalText() ([]byte, error) {
	if u.marshalError != nil {
		return nil, u.marshalError
	}
	return []byte(u.string()), nil
}

func (u Uuid) string() string {
	uuid, err := uuid.FromBytes([]byte(u.bytes[:]))
	if err != nil {
		return ""
	}
	return uuid.String()
}

func (u *Uuid) UnmarshalText(data []byte) error {
	if string(data) == "" {
		return nil
	}

	uu, err := uuid.FromString(string(data))
	if err != nil {
		return err
	}

	*u = Uuid{bytes: uu}
	return nil
}
