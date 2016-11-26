package introspection

import (
	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
)

type introspectionSchema struct {
	types    map[string]graphql.Type
	query    graphql.Type
	mutation graphql.Type
}

type DirectiveLocation string

const (
	QUERY               DirectiveLocation = "QUERY"
	MUTATION                              = "MUTATION"
	FIELD                                 = "FIELD"
	FRAGMENT_DEFINITION                   = "FRAGMENT_DEFINITION"
	FRAGMENT_SPREAD                       = "FRAGMENT_SPREAD"
	INLINE_FRAGMENT                       = "INLINE_FRAGMENT"
)

type TypeKind string

const (
	SCALAR       TypeKind = "SCALAR"
	OBJECT                = "OBJECT"
	INTERFACE             = "INTERFACE"
	UNION                 = "UNION"
	ENUM                  = "ENUM"
	INPUT_OBJECT          = "INPUT_OBJECT"
	LIST                  = "LIST"
	NON_NULL              = "NON_NULL"
)

type InputValue struct {
	Name         string
	Description  string
	Type         Type
	DefaultValue string
}

func (s *introspectionSchema) InputValue() schemabuilder.Spec {
	return schemabuilder.Spec{
		Name: "__InputValue",
		Type: InputValue{},
	}
}

type EnumValue struct {
	Name              string
	Description       string
	IsDeprecated      bool
	DeprecationReason string
}

func (s *introspectionSchema) EnumValue() schemabuilder.Spec {
	return schemabuilder.Spec{
		Name: "__EnumValue",
		Type: EnumValue{},
	}
}

type Directive struct {
	Name        string
	Description string
	Locations   []DirectiveLocation
	Args        []InputValue
}

func (s *introspectionSchema) Directive() schemabuilder.Spec {
	return schemabuilder.Spec{
		Name: "__Directive",
		Type: Directive{},
	}
}

type Schema struct {
	Types            []Type
	QueryType        *Type
	MutationType     *Type
	SubscriptionType *Type
	Directives       []Directive
}

func (s *introspectionSchema) Schema() schemabuilder.Spec {
	return schemabuilder.Spec{
		Name: "__Schema",
		Type: Schema{},
	}
}

type Type struct {
	Inner graphql.Type `graphql:"-"`
}

func (s *introspectionSchema) Type() schemabuilder.Spec {
	spec := schemabuilder.Spec{
		Name: "__type",
		Type: Type{},
	}

	spec.FieldFunc("kind", func(t Type) TypeKind {
		switch t.Inner.(type) {
		case *graphql.Object:
			return OBJECT
		case *graphql.Scalar:
			return SCALAR
		case *graphql.List:
			return LIST
		default:
			return ""
		}
	})

	spec.FieldFunc("name", func(t Type) string {
		switch t := t.Inner.(type) {
		case *graphql.Object:
			return t.Name
		case *graphql.Scalar:
			return t.Type
		default:
			return ""
		}
	})

	spec.FieldFunc("description", func() string { return "" })
	spec.FieldFunc("interfaces", func() []Type { return nil })
	spec.FieldFunc("possibleTypes", func() []Type { return nil })
	spec.FieldFunc("inputFields", func() []InputValue { return nil })

	spec.FieldFunc("fields", func(t Type, args struct {
		IncludeDeprecated *bool
	}) []field {
		var fields []field

		switch t := t.Inner.(type) {
		case *graphql.Object:
			for name, f := range t.Fields {
				fields = append(fields, field{
					Name: name,
					Type: Type{Inner: f.Type},
				})
			}
		}

		return fields
	})

	spec.FieldFunc("ofType", func(t Type) *Type {
		switch t := t.Inner.(type) {
		case *graphql.List:
			return &Type{Inner: t.Type}
		default:
			return nil
		}
	})

	spec.FieldFunc("enumValues", func(args struct{ IncludeDeprecated *bool }) []EnumValue {
		return nil
	})

	return spec
}

type field struct {
	Name              string
	Description       string
	Args              []InputValue
	Type              Type
	IsDeprecated      bool
	DeprecationReason string
}

func (s *introspectionSchema) Field() schemabuilder.Spec {
	spec := schemabuilder.Spec{
		Name: "__field",
		Type: field{},
	}

	return spec
}

func collectTypes(typ graphql.Type, types map[string]graphql.Type) {
	switch typ := typ.(type) {
	case *graphql.Object:
		if _, ok := types[typ.Name]; ok {
			return
		}
		types[typ.Name] = typ

		for _, field := range typ.Fields {
			collectTypes(field.Type, types)
		}
	case *graphql.List:
		collectTypes(typ.Type, types)
	case *graphql.Scalar:
		if _, ok := types[typ.Type]; ok {
			return
		}
		types[typ.Type] = typ
	}
}

type query struct{}

func (s *introspectionSchema) Query() schemabuilder.Spec {
	spec := schemabuilder.Spec{
		Type: query{},
	}

	spec.FieldFunc("__schema", func() *Schema {
		var types []Type

		for _, typ := range s.types {
			types = append(types, Type{Inner: typ})
		}

		return &Schema{
			Types:        types,
			QueryType:    &Type{Inner: s.query},
			MutationType: &Type{Inner: s.mutation},
		}
	})

	spec.FieldFunc("__type", func(args struct{ Name string }) *Type {
		if typ, ok := s.types[args.Name]; ok {
			return &Type{Inner: typ}
		}
		return nil
	})

	return spec
}

type mutation struct{}

func (s *introspectionSchema) Mutation() schemabuilder.Spec {
	spec := schemabuilder.Spec{
		Type: mutation{},
	}

	return spec
}

func AddIntrospectionToSchema(schema *graphql.Schema) {
	types := make(map[string]graphql.Type)
	collectTypes(schema.Query, types)
	collectTypes(schema.Mutation, types)

	is := &introspectionSchema{
		types:    types,
		query:    schema.Query,
		mutation: schema.Mutation,
	}
	isSchema := schemabuilder.MustBuildSchema(is)

	query := schema.Query.(*graphql.Object)

	isQuery := isSchema.Query.(*graphql.Object)
	for k, v := range query.Fields {
		isQuery.Fields[k] = v
	}

	schema.Query = isQuery
}
