package main

import (
	"context"
	"errors"
	"net/http"

	"github.com/bradfitz/slice"
	"github.com/samsarahq/thunder/graphql"
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
	return schemabuilder.Spec{
		Type: Query{},
		Methods: schemabuilder.Methods{
			"messages": s.messages,
		},
	}
}

func (s *Server) messages(ctx context.Context) ([]*Message, error) {
	var result []*Message
	if err := s.db.Query(ctx, &result, nil, nil); err != nil {
		return nil, err
	}
	return result, nil
}

type Mutation struct{}

func (s *Server) Mutation() schemabuilder.Spec {
	return schemabuilder.Spec{
		Type: Mutation{},
		Methods: schemabuilder.Methods{
			"addMessage": func(ctx context.Context, args struct{ Text string }) error {
				_, err := s.db.InsertRow(ctx, &Message{Text: args.Text})
				return err
			},
			"deleteMessage": func(ctx context.Context, args struct{ Id int64 }) error {
				return s.db.DeleteRow(ctx, &Message{Id: args.Id})
			},
			"addReaction": func(ctx context.Context, args struct {
				MessageId int64
				Reaction  string
			}) error {
				if _, ok := reactionTypes[args.Reaction]; !ok {
					return errors.New("reaction not allowed")
				}

				_, err := s.db.InsertRow(ctx, &ReactionInstance{MessageId: args.MessageId, Reaction: args.Reaction})
				return err
			},
		},
	}
}

func (s *Server) Message() schemabuilder.Spec {
	return schemabuilder.Spec{
		Type: Message{},
		Methods: schemabuilder.Methods{
			"reactions": s.messageReactions,
		},
	}
}

func (s *Server) messageReactions(ctx context.Context, m *Message) ([]*Reaction, error) {
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

	http.Handle("/graphql", graphql.Handler(graphqlSchema))
	if err := http.ListenAndServe(":3030", nil); err != nil {
		panic(err)
	}
}
