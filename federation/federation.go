package main

import (
	"context"
	"fmt"

	"github.com/samsarahq/thunder/batch"
	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
)

type Foo struct {
	Name string
}

type Bar struct {
	Id int64
}

func schema1() *schemabuilder.Schema {
	schema := schemabuilder.NewSchema()

	query := schema.Query()
	query.FieldFunc("f", func() *Foo {
		return &Foo{
			Name: "jimbob",
		}
	})
	query.FieldFunc("fff", func() []*Foo {
		return []*Foo{
			{
				Name: "jimbo",
			},
			{
				Name: "bob",
			},
		}
	})

	foo := schema.Object("foo", Foo{})
	foo.BatchFieldFunc("hmm", func(ctx context.Context, in map[batch.Index]*Foo) (map[batch.Index]string, error) {
		out := make(map[batch.Index]string)
		for i, foo := range in {
			out[i] = foo.Name + "!!!"
		}
		return out, nil
	})
	foo.FieldFunc("federationKey", func(f *Foo) string {
		return f.Name
	})

	foo.FieldFunc("nest", func(f *Foo) *Foo {
		return f
	})

	schema.Query().FieldFunc("barsFromFederationKeys", func(args struct{ Keys []int64 }) []*Bar {
		bars := make([]*Bar, 0, len(args.Keys))
		for _, key := range args.Keys {
			bars = append(bars, &Bar{Id: key})
		}
		return bars
	})

	bar := schema.Object("bar", Bar{})
	bar.FieldFunc("baz", func(b *Bar) string {
		return fmt.Sprint(b.Id)
	})

	return schema
}

func schema2() *schemabuilder.Schema {
	schema := schemabuilder.NewSchema()

	schema.Query().FieldFunc("foosFromFederationKeys", func(args struct{ Keys []string }) []*Foo {
		foos := make([]*Foo, 0, len(args.Keys))
		for _, key := range args.Keys {
			foos = append(foos, &Foo{Name: key})
		}
		return foos
	})

	foo := schema.Object("foo", Foo{})

	// XXX: require schema.Key

	// XXX: how do we expose foo? ... flatten is annoying

	foo.FieldFunc("ok", func(ctx context.Context, in *Foo) (int, error) {
		return len(in.Name), nil
	})

	foo.FieldFunc("bar", func(in *Foo) *Bar {
		return &Bar{
			Id: int64(len(in.Name)*2 + 4),
		}
	})

	bar := schema.Object("bar", Bar{})
	bar.FieldFunc("federationKey", func(b *Bar) int64 {
		return b.Id
	})

	return schema
}

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
							Args: nil,                         // xxx
							Type: byName[getName(field.Type)], // XXX
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
		Query:  byName["Query"], // XXX
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
