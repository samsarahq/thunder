package main

import (
	"context"
	"errors"
	"net/http"

	"github.com/bradfitz/slice"
	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/introspection"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/samsarahq/thunder/livesql"
	"github.com/samsarahq/thunder/sqlgen"
)

var reactionTypes = map[string]bool{
	":)": true,
	":(": true,
}

type Message struct {
	Id   int64 `sql:",primary" graphql:",key"`
	Text string
}

type ReactionInstance struct {
	Id        int64 `sql:",primary"`
	MessageId int64
	Reaction  string
}

type Reaction struct {
	Reaction string `graphql:",key"`
	Count    int
}

type Server struct {
	db *livesql.LiveDB
}

type Query struct{}

func (s *Server) Query() schemabuilder.Spec {
	spec := schemabuilder.Spec{
		Type: Query{},
	}
	spec.FieldFunc("messages", func(ctx context.Context) ([]*Message, error) {
		var result []*Message
		if err := s.db.Query(ctx, &result, nil, nil); err != nil {
			return nil, err
		}
		return result, nil
	})
	return spec
}

type Mutation struct{}

func (s *Server) Mutation() schemabuilder.Spec {
	spec := schemabuilder.Spec{
		Type: Mutation{},
	}
	spec.FieldFunc("addMessage", func(ctx context.Context, args struct{ Text string }) error {
		_, err := s.db.InsertRow(ctx, &Message{Text: args.Text})
		return err
	})
	spec.FieldFunc("deleteMessage", func(ctx context.Context, args struct{ Id int64 }) error {
		return s.db.DeleteRow(ctx, &Message{Id: args.Id})
	})
	spec.FieldFunc("addReaction", func(ctx context.Context, args struct {
		MessageId int64
		Reaction  string
	}) error {
		if _, ok := reactionTypes[args.Reaction]; !ok {
			return errors.New("reaction not allowed")
		}
		_, err := s.db.InsertRow(ctx, &ReactionInstance{MessageId: args.MessageId, Reaction: args.Reaction})
		return err
	})
	return spec
}

func (s *Server) Message() schemabuilder.Spec {
	spec := schemabuilder.Spec{
		Type: Message{},
	}
	spec.FieldFunc("reactions", func(ctx context.Context, m *Message) ([]*Reaction, error) {
		reactions := make(map[string]*Reaction)
		for reactionType := range reactionTypes {
			reactions[reactionType] = &Reaction{
				Reaction: reactionType,
			}
		}

		var instances []*ReactionInstance
		if err := s.db.Query(ctx, &instances, sqlgen.Filter{"message_id": m.Id}, nil); err != nil {
			return nil, err
		}
		for _, instance := range instances {
			reactions[instance.Reaction].Count++
		}

		var result []*Reaction
		for _, reaction := range reactions {
			result = append(result, reaction)
		}
		slice.Sort(result, func(a, b int) bool { return result[a].Reaction < result[b].Reaction })

		return result, nil
	})
	return spec
}

func main() {
	sqlgenSchema := sqlgen.NewSchema()
	sqlgenSchema.MustRegisterType("messages", sqlgen.AutoIncrement, Message{})
	sqlgenSchema.MustRegisterType("reaction_instances", sqlgen.AutoIncrement, ReactionInstance{})

	liveDB, err := livesql.Open("localhost", 3307, "root", "", "chat", sqlgenSchema)
	if err != nil {
		panic(err)
	}

	server := &Server{
		db: liveDB,
	}
	graphqlSchema := schemabuilder.MustBuildSchema(server)

	introspection.AddIntrospectionToSchema(graphqlSchema)

	http.Handle("/graphql", graphql.Handler(graphqlSchema))
	if err := http.ListenAndServe(":3030", nil); err != nil {
		panic(err)
	}
}
