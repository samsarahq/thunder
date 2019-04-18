package graphql

import (
	"context"
	"encoding/json"
)

func NewObjectWriter(parent OutputWriter) *ObjectWriter {
	return &ObjectWriter{
		parent: parent,
	}
}

type ObjectWriter struct {
	parent OutputWriter
	res    interface{}
	err    error
}

func (o *ObjectWriter) MarshalJSON() ([]byte, error) {
	return json.Marshal(o.res)
}

func (o *ObjectWriter) Fill(res interface{}) {
	o.res = res
}

func (o *ObjectWriter) Fail(err error) {
	if o.parent != nil {
		o.parent.Fail(err)
		return
	}
	o.err = err
}

type OutputWriter interface {
	json.Marshaler

	Fill(interface{})
	Fail(error)
}

type ExecutionUnit struct {
	Ctx          context.Context
	Sources      []interface{}
	Field        *Field
	Destinations []OutputWriter
	Selection    *Selection
}

type BatchResolver func(unit *ExecutionUnit) []*ExecutionUnit
