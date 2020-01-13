package federation

import (
	"errors"
	"fmt"
	"sort"

	"github.com/samsarahq/thunder/graphql"
)

// serviceSchemas holds all schemas for all of versions of
// all executors services. It is a map from service name
// and version to schema.
type serviceSchemas map[string]map[string]*introspectionQueryResult

// FieldInfo holds federation-specific information for
// graphql.Fields used to plan and execute queries.
type FieldInfo struct {
	// Services is the set of all services that can resolve this
	// field. If a service has multiple versions, all versions
	// must be able to resolve the field.
	Services map[string]bool
}

// SchemaWithFederationInfo holds a graphql.Schema along with
// federtion-specific annotations per field.
type SchemaWithFederationInfo struct {
	Schema *graphql.Schema
	// Fields is a map of fields to services which they belong to
	Fields map[*graphql.Field]*FieldInfo
}

// convertVersionedSchemas takes schemas for all of versions of
// all executors services and generates a single merged schema
// annotated with mapping from field to all services that know
// how to resolve the field
func convertVersionedSchemas(schemas serviceSchemas) (*SchemaWithFederationInfo, error) {
	serviceNames := make([]string, 0, len(schemas))
	for service := range schemas {
		serviceNames = append(serviceNames, service)
	}
	sort.Strings(serviceNames)

	serviceSchemasByName := make(map[string]*introspectionQueryResult)

	// Finds the intersection of different version of the schemas
	var serviceSchemas []*introspectionQueryResult
	for _, service := range serviceNames {
		versions := schemas[service]

		versionNames := make([]string, 0, len(versions))
		for version := range versions {
			versionNames = append(versionNames, version)
		}
		sort.Strings(versionNames)

		var versionSchemas []*introspectionQueryResult
		for _, version := range versionNames {
			versionSchemas = append(versionSchemas, versions[version])
		}

		serviceSchema, err := mergeSchemaSlice(versionSchemas, Intersection)
		if err != nil {
			return nil, err
		}

		serviceSchemasByName[service] = serviceSchema

		serviceSchemas = append(serviceSchemas, serviceSchema)
	}

	// Finds the union of all the schemas from different executor services
	merged, err := mergeSchemaSlice(serviceSchemas, Union)
	if err != nil {
		return nil, err
	}

	types, err := parseSchema(merged)
	if err != nil {
		return nil, err
	}

	fieldInfos := make(map[*graphql.Field]*FieldInfo)
	for _, service := range serviceNames {
		for _, typ := range serviceSchemasByName[service].Schema.Types {
			if typ.Kind == "OBJECT" {
				obj := types[typ.Name].(*graphql.Object)

				for _, field := range typ.Fields {
					f := obj.Fields[field.Name]

					info, ok := fieldInfos[f]
					if !ok {
						info = &FieldInfo{
							Services: map[string]bool{},
						}
						fieldInfos[f] = info
					}
					info.Services[service] = true
				}
			}
		}
	}

	return &SchemaWithFederationInfo{
		Schema: &graphql.Schema{
			Query:    types["Query"],
			Mutation: types["Mutation"],
		},
		Fields: fieldInfos,
	}, nil
}

// convertSchema annotates the schema with federation information vt
// mapping fields to the corresponding services.
func convertSchema(schemas map[string]*introspectionQueryResult) (*SchemaWithFederationInfo, error) {
	versionedSchemas := make(serviceSchemas)
	for service, schema := range schemas {
		versionedSchemas[service] = map[string]*introspectionQueryResult{
			"": schema,
		}
	}
	return convertVersionedSchemas(versionedSchemas)
}

// lookupTypeRef maps the a introspected type to a graphql type
func lookupType(t *introspectionTypeRef, all map[string]graphql.Type) (*introspectionTypeRef, error) {
	if t == nil {
		return nil, errors.New("malformed typeref")
	}
	switch t.Kind {
	case "SCALAR", "OBJECT", "UNION", "INPUT_OBJECT", "ENUM":
		return t, nil
	case "LIST":
		return lookupType(t.OfType, all)
	case "NON_NULL":
		return lookupType(t.OfType, all)
	default:
		return nil, fmt.Errorf("unknown type kind %s", t.Kind)
	}
}

// lookupTypeRef maps the a introspected type to a graphql type
func lookupTypeRef(t *introspectionTypeRef, all map[string]graphql.Type) (graphql.Type, error) {
	if t == nil {
		return nil, errors.New("malformed typeref")
	}

	switch t.Kind {
	case "SCALAR", "OBJECT", "UNION", "INPUT_OBJECT", "ENUM":
		// TODO: enforce type?
		typ, ok := all[t.Name]
		if !ok {
			return nil, fmt.Errorf("type %s not found among top-level types", t.Name)
		}
		return typ, nil

	case "LIST":
		inner, err := lookupTypeRef(t.OfType, all)
		if err != nil {
			return nil, err
		}
		return &graphql.List{
			Type: inner,
		}, nil

	case "NON_NULL":
		inner, err := lookupTypeRef(t.OfType, all)
		if err != nil {
			return nil, err
		}
		return &graphql.NonNull{
			Type: inner,
		}, nil

	default:
		return nil, fmt.Errorf("unknown type kind %s", t.Kind)
	}
}

// parseInputFields maps a list of input types to a list of graphql types
func parseInputFields(source []introspectionInputField, all map[string]graphql.Type) (map[string]graphql.Type, error) {
	fields := make(map[string]graphql.Type)

	for _, field := range source {
		// Validate the inputType is valid
		rawType, err := lookupType(field.Type, all)
		if err != nil {
			return nil, fmt.Errorf("type %s not found", rawType.Name)
		}
		switch rawType.Kind {
		case "INPUT_OBJECT", "SCALAR":
		default:
			return nil, fmt.Errorf("input field %s has bad typ: %s", field.Name, rawType.Kind)
		}

		inputType, err := lookupTypeRef(field.Type, all)
		if err != nil {
			return nil, fmt.Errorf("field %s has bad typ: %v", field.Name, err)
		}
		fields[field.Name] = inputType
	}

	return fields, nil
}

// parseSchema takes the introspected schema, validates the types,
// and maps every field to the graphql types
func parseSchema(schema *introspectionQueryResult) (map[string]graphql.Type, error) {
	all := make(map[string]graphql.Type)

	for _, typ := range schema.Schema.Types {
		if _, ok := all[typ.Name]; ok {
			return nil, fmt.Errorf("duplicate type %s", typ.Name)
		}

		switch typ.Kind {
		case "OBJECT":
			all[typ.Name] = &graphql.Object{
				Name: typ.Name,
			}

		case "INPUT_OBJECT":
			all[typ.Name] = &graphql.InputObject{
				Name: typ.Name,
			}

		case "SCALAR":
			all[typ.Name] = &graphql.Scalar{
				Type: typ.Name,
			}

		case "UNION":
			all[typ.Name] = &graphql.Union{
				Name: typ.Name,
			}

		case "ENUM":
			all[typ.Name] = &graphql.Enum{
				Type: typ.Name,
			}

		default:
			return nil, fmt.Errorf("unknown type kind %s", typ.Kind)
		}
	}

	// Initialize barebone types
	for _, typ := range schema.Schema.Types {
		switch typ.Kind {
		case "OBJECT":
			fields := make(map[string]*graphql.Field)
			for _, field := range typ.Fields {
				fieldTyp, err := lookupTypeRef(field.Type, all)
				if err != nil {
					return nil, fmt.Errorf("typ %s field %s has bad typ: %v",
						typ.Name, field.Name, err)
				}

				parsed, err := parseInputFields(field.Args, all)
				if err != nil {
					return nil, fmt.Errorf("field %s input: %v", field.Name, err)
				}

				fields[field.Name] = &graphql.Field{
					Args: parsed,
					Type: fieldTyp,
				}
			}

			all[typ.Name].(*graphql.Object).Fields = fields

		case "INPUT_OBJECT":
			parsed, err := parseInputFields(typ.InputFields, all)
			if err != nil {
				return nil, fmt.Errorf("typ %s: %v", typ.Name, err)
			}

			all[typ.Name].(*graphql.InputObject).InputFields = parsed

		case "UNION":
			types := make(map[string]*graphql.Object)
			for _, other := range typ.PossibleTypes {
				if other.Kind != "OBJECT" {
					return nil, fmt.Errorf("typ %s has possible typ not OBJECT: %v", typ.Name, other)
				}
				typ, ok := all[other.Name].(*graphql.Object)
				if !ok {
					return nil, fmt.Errorf("typ %s possible typ %s does not refer to obj", typ.Name, other.Name)
				}
				types[typ.Name] = typ
			}

			all[typ.Name].(*graphql.Union).Types = types

		case "ENUM":
			// XXX: introspection relies on the EnumValues map.
			reverseMap := make(map[interface{}]string)
			values := make([]string, 0, len(typ.EnumValues))
			for _, value := range typ.EnumValues {
				values = append(values, value.Name)
				reverseMap[value.Name] = value.Name
			}

			enum := all[typ.Name].(*graphql.Enum)
			enum.Values = values
			enum.ReverseMap = reverseMap

		case "SCALAR":
			// pass

		default:
			return nil, fmt.Errorf("unknown type kind %s", typ.Kind)
		}
	}

	return all, nil
}

// XXX: for types missing __federation, take intersection?

// XXX: for (merged) unions, make sure we only send possible types
// to each service

// TODO: support descriptions in merging
