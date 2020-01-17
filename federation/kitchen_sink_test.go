package federation

import (
	"context"
	"fmt"
	"github.com/samsarahq/thunder/batch"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
)

type Enum int

type Foo struct {
	Name string
}

type Bar struct {
	Id int64
}

type FooOrBar struct {
	schemabuilder.Union
	*Foo
	*Bar
}

type Pair struct {
	A, B int64
}

func buildTestSchema1() *schemabuilder.Schema {
	schema := schemabuilder.NewSchema()

	query := schema.Query()
	query.FieldFunc("s1f", func() *Foo {
		return &Foo{
			Name: "jimbob",
		}
	})
	query.FieldFunc("s1fff", func() []*Foo {
		return []*Foo{
			{
				Name: "jimbo",
			},
			{
				Name: "bob",
			},
		}
	})

	query.FieldFunc("s1echo", func(args struct {
		Foo      string
		Required Pair
		Optional *int64
	}) string {
		return fmt.Sprintf("%s %v %v", args.Foo, args.Required, args.Optional)
	})

	schema.Enum(Enum(1), map[string]Enum{
		"one": 1,
	})

	mutation := schema.Mutation()

	mutation.FieldFunc("s1addFoo", func(args struct{ Name string }) *Foo {
		return &Foo{
			Name: args.Name,
		}
	})

	foo := schema.Object("Foo", Foo{})
	foo.Federation(func(f *Foo) string {
		return f.Name
	})
	foo.BatchFieldFunc("s1hmm", func(ctx context.Context, in map[batch.Index]*Foo) (map[batch.Index]string, error) {
		out := make(map[batch.Index]string)
		for i, foo := range in {
			out[i] = foo.Name + "!!!"
		}
		return out, nil
	})
	foo.FieldFunc("s1nest", func(f *Foo) *Foo {
		return f
	})
	foo.FieldFunc("s1enum", func(f *Foo) Enum {
		return Enum(1)
	})

	schema.Federation().FieldFunc("Bar", func(args struct{ Keys []int64 }) []*Bar {
		bars := make([]*Bar, 0, len(args.Keys))
		for _, key := range args.Keys {
			bars = append(bars, &Bar{Id: key})
		}
		return bars
	})

	bar := schema.Object("Bar", Bar{})
	bar.FieldFunc("s1baz", func(b *Bar) string {
		return fmt.Sprint(b.Id)
	})

	query.FieldFunc("s1both", func() []FooOrBar {
		return []FooOrBar{
			{
				Foo: &Foo{
					Name: "this is the foo",
				},
			},
			{
				Bar: &Bar{
					Id: 1234,
				},
			},
		}
	})

	return schema
}

func buildTestSchema2() *schemabuilder.Schema {
	schema := schemabuilder.NewSchema()

	schema.Federation().FieldFunc("Foo", func(args struct{ Keys []string }) []*Foo {
		foos := make([]*Foo, 0, len(args.Keys))
		for _, key := range args.Keys {
			foos = append(foos, &Foo{Name: key})
		}
		return foos
	})

	schema.Query().FieldFunc("s2root", func() string {
		return "hello"
	})

	foo := schema.Object("Foo", Foo{})

	foo.FieldFunc("s2ok", func(ctx context.Context, in *Foo) (int, error) {
		return len(in.Name), nil
	})

	foo.FieldFunc("s2ok2", func(in *Foo) (int, error) {
		return len(in.Name), nil
	})

	foo.FieldFunc("s2bar", func(in *Foo) *Bar {
		return &Bar{
			Id: int64(len(in.Name)*2 + 4),
		}
	})

	foo.FieldFunc("s2nest", func(f *Foo) *Foo {
		return f
	})

	bar := schema.Object("Bar", Bar{})
	bar.Federation(func(b *Bar) int64 {
		return b.Id
	})

	return schema
}
