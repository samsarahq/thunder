package a

import (
	"github.com/samsarahq/thunder/batch"
)

type Server struct{}

type objectType struct{}

func schema() *Schema {
	server := &Server{}
	schema := NewSchema()
	server.Register(schema)
	return schema
}

func (s *Server) Register(schema *Schema) {
	s.registerRoot(schema)
}

func (s *Server) registerRoot(schema *Schema) {
	object := schema.Query()
	object.FieldFunc("listEntryPtr", s.listEntryPtr)
	object.FieldFunc("listEntryPtrDone", s.listEntryPtr)
	object.FieldFunc(
		"listEntryPtr",
		s.listEntryPtr,
	)
	object.FieldFunc("listEntryNonPtr", s.listEntryNonPtr)
	object.FieldFunc("listEntryCustomTypePtr", s.listEntryCustomTypePtr)
	object.FieldFunc("listEntryCustomSliceTypePtr", s.listEntryCustomSliceTypePtr)
	object.FieldFunc("listEntryPtrAnonymous", func() []*string { return []*string{} })
	object.FieldFunc("listEntryNonPtrAnonymous", func() []string { return []string{} })
	object.FieldFunc("listEntryPtrNormal", listEntryPtr)
	object.FieldFunc("listEntryNonPtrNormal", listEntryNonPtr)
	object.FieldFunc("listEntryCustomTypePtrNormal", listEntryCustomTypePtr)
	object.FieldFunc("listEntryCustomSliceTypePtrNormal", listEntryCustomSliceTypePtr)

	object.FieldFunc("batchListEntryPtr", s.batchListEntryPtr)
	object.FieldFunc("batchListEntryNonPtr", s.batchListEntryNonPtr)
	object.FieldFunc("batchListEntryPtrNormal", batchListEntryPtr)
	object.FieldFunc("batchListEntryNonPtrNormal", batchListEntryNonPtr)
	object.FieldFunc("batchListEntryPtrAnonymous", func() map[batch.Index][]*string { return map[batch.Index][]*string{} })
	object.FieldFunc("batchListEntryNonPtrAnonymous", func() map[batch.Index][]string { return map[batch.Index][]string{} })
	object.FieldFunc("batchListEntryCustomSliceTypePtr", s.batchListEntryCustomSliceTypePtr)
	object.FieldFunc("batchListEntryCustomSliceTypePtrNormal", batchListEntryCustomSliceTypePtr)
}

type myStringPtr *string

func (s *Server) listEntryCustomTypePtr() []myStringPtr {
	return []myStringPtr{}
}

type myStringPtrSlice []*string

func (s *Server) listEntryCustomSliceTypePtr() myStringPtrSlice {
	return myStringPtrSlice{}
}

type myMapPtr map[batch.Index]myStringPtrSlice

func (s *Server) batchListEntryCustomSliceTypePtr() myMapPtr {
	return myMapPtr{}
}

func (s *Server) listEntryPtr() []*string {
	return []*string{}
}

func (s *Server) listEntryNonPtr() []string {
	return []string{}
}

func (s *Server) batchListEntryPtr() map[batch.Index][]*string {
	return map[batch.Index][]*string{}
}

func (s *Server) batchListEntryNonPtr() map[batch.Index][]string {
	return map[batch.Index][]string{}
}

func listEntryPtr() []*string {
	return []*string{}
}

func listEntryNonPtr() []string {
	return []string{}
}

func batchListEntryPtr() map[batch.Index][]*string {
	return map[batch.Index][]*string{}
}

func batchListEntryNonPtr() map[batch.Index][]string {
	return map[batch.Index][]string{}
}

func listEntryCustomTypePtr() []myStringPtr {
	return []myStringPtr{}
}

func listEntryCustomSliceTypePtr() myStringPtrSlice {
	return myStringPtrSlice{}
}

func batchListEntryCustomSliceTypePtr() myMapPtr {
	return myMapPtr{}
}
