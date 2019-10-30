package federation

import (
	"github.com/samsarahq/thunder/graphql"
)

func getName(t *TypeRef) string {
	if t == nil {
		panic("nil")
	}

	switch t.Kind {
	case "SCALAR", "OBJECT":
		return t.Name
	case "LIST", "NON_NULL":
		return getName(t.OfType)
	default:
		panic("help")
	}
}

type TypeRef struct {
	Kind   string   `json:"kind"`
	Name   string   `json:"name"`
	OfType *TypeRef `json:"ofType"`
}

type IntrospectionQuery struct {
	Schema struct {
		Types []struct {
			Name   string `json:"name"`
			Kind   string `json:"kind"`
			Fields []struct {
				Name string   `json:"name"`
				Type *TypeRef `json:"type"`
			} `json:"fields"`
		} `json:"types"`
	} `json:"__schema"`
}

func convertSchema(schemas map[string]IntrospectionQuery) SchemaWithFederationInfo {
	byName := make(map[string]*graphql.Object)
	all := make(map[string]graphql.Type)
	fieldInfos := make(map[*graphql.Field]*FieldInfo)

	for _, schema := range schemas {
		for _, typ := range schema.Schema.Types {
			switch typ.Kind {
			case "OBJECT":
				if _, ok := byName[typ.Name]; !ok {
					byName[typ.Name] = &graphql.Object{
						Name:   typ.Name,
						Fields: make(map[string]*graphql.Field),
					}
					all[typ.Name] = byName[typ.Name]
				}

			case "SCALAR":
				all[typ.Name] = &graphql.Scalar{
					Type: typ.Name,
				}

			default:
				// XXX
			}
		}
	}

	for service, schema := range schemas {
		for _, typ := range schema.Schema.Types {
			switch typ.Kind {
			case "OBJECT":
				obj := byName[typ.Name]

				for _, field := range typ.Fields {
					f, ok := obj.Fields[field.Name]
					if !ok {
						f = &graphql.Field{
							Args: nil,                      // xxx
							Type: all[getName(field.Type)], // XXX
						}
						obj.Fields[field.Name] = f
						fieldInfos[f] = &FieldInfo{
							Service:  service,
							Services: map[string]bool{},
						}
					}

					fieldInfos[f].Services[service] = true
				}

			default:
				// XXX
			}
		}
	}

	return SchemaWithFederationInfo{
		Schema: &graphql.Schema{
			Query:    byName["Query"],    // XXX
			Mutation: byName["Mutation"], // XXX
		},
		Fields: fieldInfos,
	}
}

// schema.Extend()

// XXX: any types you return you must have the definition for...
//
//   how do we enforce that?? some compile time check that crosses package
//   boundaries and spots Object() (or whatever) calls that are automatic in some
//   package and not in another?
//
//   could not do magic anymore and require an explicit "schema.Object" call for
//   any types returned... maybe with schema.AutoObject("") to handle automatic
//   cases?
//
// XXX: could not allow schemabuilder auto objects outside of packages? seems nice.
// }
