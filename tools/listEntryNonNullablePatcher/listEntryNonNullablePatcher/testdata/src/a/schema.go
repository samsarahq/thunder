// This file is a minimal copy of
// github.com/samsarahq/thunder/graphql/schemabuilder for testing.
//
// We make a toy copy of schemabuilder inside package a because the analyzer
// tests only work within one package.
package a

type Schema struct{}

// NewSchema creates a new schema.
func NewSchema() *Schema {
	return &Schema{}
}

func (s *Schema) Object(name string, typ interface{}) *Object {
	return &Object{}
}

type query struct{}

func (s *Schema) Query() *Object {
	return s.Object("Query", query{})
}

type mutation struct{}

func (s *Schema) Mutation() *Object {
	return s.Object("mutation", mutation{})
}

type FieldFuncOption interface {
	Apply()
}

type Object struct{}

func (s *Object) FieldFunc(name string, f interface{}, options ...FieldFuncOption) {}
