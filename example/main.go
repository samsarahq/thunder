package main

import (
	"context"
	"net/http"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/livesql"
	"github.com/samsarahq/thunder/sqlgen"
)

type Server struct {
	db *livesql.LiveDB
}

type Message struct {
	Id   int64 `sql:",primary"`
	Text string
}

func (s *Server) Message() graphql.Spec {
	return graphql.Spec{
		Type: Message{},
		Key:  "id",
	}
}

func (s *Server) messages(ctx context.Context) ([]*Message, error) {
	var result []*Message
	if err := s.db.Query(ctx, &result, nil, nil); err != nil {
		return nil, err
	}
	return result, nil
}

type Query struct{}

func (s *Server) Query() graphql.Spec {
	return graphql.Spec{
		Type: Query{},
		Methods: graphql.Methods{
			"messages": s.messages,
		},
	}
}

type Mutation struct{}

func (s *Server) Mutation() graphql.Spec {
	return graphql.Spec{
		Type: Mutation{},
		Methods: graphql.Methods{
			"addMessage": func(ctx context.Context, args struct{ Text string }) error {
				_, err := s.db.InsertRow(ctx, &Message{Text: args.Text})
				return err
			},
			"deleteMessage": func(ctx context.Context, args struct{ Id int64 }) error {
				return s.db.DeleteRow(ctx, &Message{Id: args.Id})
			},
		},
	}
}

func main() {
	sqlgenSchema := sqlgen.NewSchema()
	sqlgenSchema.MustRegisterType("messages", sqlgen.AutoIncrement, Message{})

	liveDB, err := livesql.Open("localhost", 3307, "root", "", "chat", sqlgenSchema)
	if err != nil {
		panic(err)
	}

	server := &Server{
		db: liveDB,
	}
	graphqlSchema := graphql.MustBuildSchema(server)

	http.Handle("/graphql", graphql.Handler(graphqlSchema))
	if err := http.ListenAndServe(":3030", nil); err != nil {
		panic(err)
	}
}
