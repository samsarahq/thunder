package introspection

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
)

type introspection struct {
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

func (s *introspection) registerInputValue(schema *schemabuilder.Schema) {
	schema.Object("__InputValue", InputValue{})
}

type EnumValue struct {
	Name              string
	Description       string
	IsDeprecated      bool
	DeprecationReason string
}

func (s *introspection) registerEnumValue(schema *schemabuilder.Schema) {
	schema.Object("__EnumValue", EnumValue{})
}

type Directive struct {
	Name        string
	Description string
	Locations   []DirectiveLocation
	Args        []InputValue
}

func (s *introspection) registerDirective(schema *schemabuilder.Schema) {
	schema.Object("__Directive", Directive{})
}

type Schema struct {
	Types            []Type
	QueryType        *Type
	MutationType     *Type
	SubscriptionType *Type
	Directives       []Directive
}

func (s *introspection) registerSchema(schema *schemabuilder.Schema) {
	schema.Object("__Schema", Schema{})
}

type Type struct {
	Inner graphql.Type `graphql:"-"`
}

func (s *introspection) registerType(schema *schemabuilder.Schema) {
	object := schema.Object("__Type", Type{})

	object.FieldFunc("kind", func(t Type) TypeKind {
		switch t.Inner.(type) {
		case *graphql.Object:
			return OBJECT
		case *graphql.Scalar:
			return SCALAR
		case *graphql.List:
			return LIST
		case *graphql.InputObject:
			return INPUT_OBJECT
		default:
			return ""
		}
	})

	object.FieldFunc("name", func(t Type) string {
		switch t := t.Inner.(type) {
		case *graphql.Object:
			return t.Name
		case *graphql.Scalar:
			return t.Type
		case *graphql.InputObject:
			return t.Name
		default:
			return ""
		}
	})

	object.FieldFunc("description", func(t Type) string {
		switch t := t.Inner.(type) {
		case *graphql.Object:
			return t.Description
		default:
			return ""
		}
	})

	object.FieldFunc("interfaces", func() []Type { return nil })
	object.FieldFunc("possibleTypes", func() []Type { return nil })

	object.FieldFunc("inputFields", func(t Type) []InputValue {
		var fields []InputValue

		switch t := t.Inner.(type) {
		case *graphql.InputObject:
			for name, f := range t.InputFields {
				fields = append(fields, InputValue{
					Name: name,
					Type: Type{Inner: f},
				})
			}
		}

		sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
		return fields
	})

	object.FieldFunc("fields", func(t Type, args struct {
		IncludeDeprecated *bool
	}) []field {
		var fields []field

		switch t := t.Inner.(type) {
		case *graphql.Object:
			for name, f := range t.Fields {
				var args []InputValue
				for name, a := range f.Args {
					args = append(args, InputValue{
						Name: name,
						Type: Type{Inner: a},
					})
				}
				sort.Slice(args, func(i, j int) bool { return args[i].Name < args[j].Name })

				fields = append(fields, field{
					Name: name,
					Type: Type{Inner: f.Type},
					Args: args,
				})
			}
		}
		sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })

		return fields
	})

	object.FieldFunc("ofType", func(t Type) *Type {
		switch t := t.Inner.(type) {
		case *graphql.List:
			return &Type{Inner: t.Type}
		default:
			return nil
		}
	})

	object.FieldFunc("enumValues", func(args struct{ IncludeDeprecated *bool }) []EnumValue {
		return nil
	})
}

type field struct {
	Name              string
	Description       string
	Args              []InputValue
	Type              Type
	IsDeprecated      bool
	DeprecationReason string
}

func (s *introspection) registerField(schema *schemabuilder.Schema) {
	schema.Object("__Field", field{})
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

			for _, arg := range field.Args {
				collectTypes(arg, types)
			}
		}

	case *graphql.List:
		collectTypes(typ.Type, types)

	case *graphql.Scalar:
		if _, ok := types[typ.Type]; ok {
			return
		}
		types[typ.Type] = typ

	case *graphql.InputObject:
		if _, ok := types[typ.Name]; ok {
			return
		}
		types[typ.Name] = typ

		for _, field := range typ.InputFields {
			collectTypes(field, types)
		}
	}
}

func (s *introspection) registerQuery(schema *schemabuilder.Schema) {
	object := schema.Query()

	object.FieldFunc("__schema", func() *Schema {
		var types []Type

		for _, typ := range s.types {
			types = append(types, Type{Inner: typ})
		}
		sort.Slice(types, func(i, j int) bool { return types[i].Inner.String() < types[j].Inner.String() })

		return &Schema{
			Types:        types,
			QueryType:    &Type{Inner: s.query},
			MutationType: &Type{Inner: s.mutation},
		}
	})

	object.FieldFunc("__type", func(args struct{ Name string }) *Type {
		if typ, ok := s.types[args.Name]; ok {
			return &Type{Inner: typ}
		}
		return nil
	})
}

func (s *introspection) registerMutation(schema *schemabuilder.Schema) {
	schema.Mutation()
}

func (s *introspection) schema() *graphql.Schema {
	schema := schemabuilder.NewSchema()

	s.registerDirective(schema)
	s.registerEnumValue(schema)
	s.registerField(schema)
	s.registerInputValue(schema)
	s.registerMutation(schema)
	s.registerQuery(schema)
	s.registerSchema(schema)
	s.registerType(schema)

	return schema.MustBuild()
}

func AddIntrospectionToSchema(schema *graphql.Schema) {
	types := make(map[string]graphql.Type)
	collectTypes(schema.Query, types)
	collectTypes(schema.Mutation, types)

	is := &introspection{
		types:    types,
		query:    schema.Query,
		mutation: schema.Mutation,
	}
	isSchema := is.schema()

	query := schema.Query.(*graphql.Object)

	isQuery := isSchema.Query.(*graphql.Object)
	for k, v := range query.Fields {
		isQuery.Fields[k] = v
	}

	schema.Query = isQuery
}

// ComputeSchemaJSON returns the result of executing a GraphQL introspection
// query.
func ComputeSchemaJSON(schemaBuilderSchema schemabuilder.Schema) ([]byte, error) {
	schema := schemaBuilderSchema.MustBuild()
	AddIntrospectionToSchema(schema)

	selectionSet, err := graphql.Parse(introspectionQuery, map[string]interface{}{})
	if err != nil {
		return nil, err
	}

	if err := graphql.PrepareQuery(schema.Query, selectionSet); err != nil {
		return nil, err
	}

	executor := graphql.Executor{}
	value, err := executor.Execute(context.Background(), schema.Query, nil, selectionSet)
	if err != nil {
		return nil, err
	}

	return json.MarshalIndent(value, "", "  ")
}
