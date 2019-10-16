package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/davecgh/go-spew/spew"
	"github.com/samsarahq/thunder/graphql"

	"github.com/samsarahq/thunder/batch"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
)

type Foo struct {
	Name string
}

func schema1() *schemabuilder.Schema {
	schema := schemabuilder.NewSchema()

	query := schema.Query()
	query.FieldFunc("f", func() *Foo {
		return &Foo{
			Name: "jimbob",
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

	return schema
}

func schema2() *schemabuilder.Schema {
	schema := schemabuilder.NewSchema()

	schema.Query().FieldFunc("q", func() *Foo { return &Foo{Name: "carl"} })

	foo := schema.Object("foo", Foo{})

	// XXX: require schema.Key

	// XXX: how do we expose foo? ... flatten is annoying

	foo.FieldFunc("ok", func(ctx context.Context, in *Foo) (int, error) {
		return len(in.Name), nil
	})

	return schema
}

func walkTypes(schema *graphql.Schema) map[string]graphql.Type {
	seen := make(map[graphql.Type]bool)
	all := make(map[string]graphql.Type)

	var visit func(t graphql.Type)
	visit = func(t graphql.Type) {
		if seen[t] {
			return
		}
		seen[t] = true

		switch t := t.(type) {
		case *graphql.Object:
			all[t.Name] = t

			for _, field := range t.Fields {
				for _, arg := range field.Args {
					visit(arg)
				}
				visit(field.Type)
			}

		case *graphql.InputObject:
			all[t.Name] = t

			for _, field := range t.InputFields {
				visit(field)
			}

		case *graphql.List:
			visit(t.Type)

		case *graphql.NonNull:
			visit(t.Type)

		case *graphql.Union:
			all[t.Name] = t

			for _, typ := range t.Types {
				visit(typ)
			}
		}
	}

	visit(schema.Query)
	visit(schema.Mutation)

	return all
}

func stitchSchemas(schemas []*graphql.Schema) *graphql.Schema {
	byName := make(map[string][]graphql.Type)

	for _, schema := range schemas {
		for name, typ := range walkTypes(schema) {
			byName[name] = append(byName[name], typ)
		}
	}

	newByName := make(map[string]graphql.Type)
	for name, types := range byName {
		typ := types[0]

		// XXX: assert identical types

		switch typ := typ.(type) {
		case *graphql.Object:
			newByName[name] = &graphql.Object{
				Name:        name,
				Description: typ.Description,
				// KeyField:
				Fields: make(map[string]*graphql.Field),
			}

		case *graphql.InputObject:
			newByName[name] = &graphql.InputObject{
				Name:        name,
				InputFields: make(map[string]graphql.Type),
			}

		case *graphql.Union:
			newByName[name] = &graphql.Union{
				Name:        name,
				Description: typ.Description,
			}

			// case *graphql.Scalar:
			// newByName[name] = typ
		}
	}

	var swizzleType func(graphql.Type) graphql.Type
	swizzleType = func(t graphql.Type) graphql.Type {
		switch t := t.(type) {
		case *graphql.Object:
			return newByName[t.Name]

		case *graphql.InputObject:
			return newByName[t.Name]

		case *graphql.List:
			return &graphql.List{
				Type: swizzleType(t.Type),
			}

		case *graphql.NonNull:
			return &graphql.NonNull{
				Type: swizzleType(t.Type),
			}

		case *graphql.Union:
			return newByName[t.Name]

		case *graphql.Scalar:
			return t
			// return newByName[t.Type]
		}
		spew.Dump(t)
		panic("help")
	}

	for name, types := range byName {
		for _, typ := range types {
			log.Println(name)
			switch typ := typ.(type) {
			case *graphql.Object:
				newTyp := newByName[name].(*graphql.Object)
				for name, field := range typ.Fields {
					log.Println(name)
					copy := *field
					copy.Type = swizzleType(field.Type)
					if copy.Type == nil {
						panic(field.Type)
					}
					newTyp.Fields[name] = &copy
				}

			case *graphql.InputObject:
				// XXX: assert identical fields

			case *graphql.Union:
				// XXX: assert identical
			}
		}
	}

	return &graphql.Schema{
		Query:    newByName["Query"],
		Mutation: newByName["Mutation"],
	}
}

func main() {
	merged := stitchSchemas([]*graphql.Schema{
		schema1().MustBuild(),
		schema2().MustBuild(),
	})

	query := graphql.MustParse(`
		{
			f {
				hmm
				ok
			}
		}
	`, map[string]interface{}{})
	if err := graphql.PrepareQuery(merged.Query, query.SelectionSet); err != nil {
		panic(err)
	}
	e := graphql.NewExecutor(
		graphql.NewImmediateGoroutineScheduler(),
	)
	res, err := e.Execute(context.Background(), merged.Query, nil, query)
	if err != nil {
		panic(err)
	}
	bytes, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		panic(err)
	}
	fmt.Print(string(bytes))

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
}
