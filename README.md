# ⚠️ Deprecated

As of Feb 15, 2023, this repository is deprecated and no longer maintained. If you are looking for a GraphQL server library in Golang, please consider [alternatives](https://graphql.org/code/#go). Thank you for your interest in Thunder.

# Thunder

Thunder is a Go framework for rapidly building powerful graphql servers.
Thunder has support for schemas automatically generated from Go types, live
queries, query batching, and more. Thunder is an open-source project from
Samsara.

[![Documentation](https://godoc.org/github.com/samsarahq/thunder?status.svg)](http://godoc.org/github.com/samsarahq/thunder)

# Feature Lightning Tour

Thunder has a number of features to make it easy to build sophisticated
schemas. This section provides a brief overview of some of them.

## Reflection-based schema building

Thunder generates resolvers automatically from Go struct types and function
definitions. For example, the `Friend` struct below gets exposed as a graphql
object type with `firstName` and `lastName` resolvers that return the fields
on the type.

```go
// Friend is a small struct representing a person.
type Friend struct {
  FirstName string
  Last string `graphql:"lastName"` // use a custom name

  Added time.Date `graphql:"-"` // don't expose over graphql
}

// FullName builds a friend's full name.
func (f *Friend) FullName() string {
  return fmt.Sprintf("%s %s", f.FirstName, f.Last)
}

// registerFriend registers custom resolvers on the Friend type.
//
// Note: registerFriend wouldn't be necessary if the type only
// had the default struct field resolvers above.
func registerFriend(schema *schemabuilder.Schema) {
  object := schema.Object("Friend", Friend{})

  // fullName is a computed field on the Friend{} object.
  object.FieldFunc("fullName", Friend.FullName)
}
```

## [Pagination](./doc/pagination.md)

## Live queries

Thunder has support for automatically updating queries using _resolver
invalidation_. With invalidation, code on the server can trigger updates on
the client using a persistent WebSocket connection.

The simplest example is a clock that updates over time. Every 10 seconds the
`time` function will be recomputed, and the latest time will be sent to the
client.

```go
// registerQuery registers the resolvers on the core graphql query type.
func registerQuery(schema *schemabuilder.Schema) {
  query := schema.Query()

  // time returns the current time.
  query.FieldFunc("time", func(ctx context.Context) string {
    // Invalidate the result of this resolver after 10 seconds.
    reactive.InvalidateAfter(ctx, 10 * time.Second)
    // Return the current time. Will be re-executed automatically.
    return time.Now().String()
  })
}
```

Using Thunder's lightweight `sqlgen` and `livesql` ORM, it's easy to write
automatically updating MySQL queries. The example below returns a live-updating
lists of posts from a database table. Whenever somebody `INSERT`s or `UPDATE`s
a row in the table, the resolver is re-executed and the latest lists of posts
is sent to the client. Behind the scenes, the `livesql` package uses MySQL's
binary replication log to detect changes to the underlying data.

```go
// A Post holds a row from the MySQL posts table.
type Post struct {
  Id    int64 `sqlgen:",primary"`
  Title string
}

// Server implements a graphql server. It has persistent handles to eg. the
// database.
type Server struct {
  db *livesql.LiveDB
}

// registerQuery registers the root query resolvers.
func (s *Server) registerQuery(schema *schemabuilder.Schema) {
  query := schema.Query()
  // posts returns all posts in the database.
  query.FieldFunc("posts", func(ctx context.Context) ([]*Post, error) {
    var posts []*Post
    if err := s.db.Query(ctx, &posts, nil, nil); err != nil {
      return nil, err
    }
    return posts, nil
  })
}
```

## Built-in parallel execution and batching

Thunder automatically runs independent resolvers in different goroutines to
quickly compute complex queries. To keep large queries efficient, Thunder has
support for built-in batching similar to Facebook's `dataloader`. With
batching, Thunder automatically combines many parallel individual calls to a
`batch.Func`'s `Invoke` function into a single call to `Many` function.

Batching is very useful when fetching related objects from a SQL database. Thunder's
`sqlgen` and `livesql` have built-in support for batching and will combine `SELECT WHERE`
statements using an `IN` clause. For example, the program below will fetch all posts and
their authors in just two queries.

```go
type Post struct {
  Id    int64 `sqlgen:",primary"`
  Title string
  AuthorId int64
}

// An Author represents a row in the authors table.
type Author struct {
  Id   int64 `sqlgen:",primary"`
  Name string
}

// registerPost registers resolvers on the Post type.
func (s *Server) registerPost(schema *schemabuilder.Schema) {
  object := schema.Object("post", Post{})
  // author return the Author object corresponding to a Post's AuthorId.
  object.FieldFunc("author", func(ctx context.Context, p *Post) (*Author, error) {
    var author *Author
    if err := s.db.QueryRow(ctx, &author, sqlgen.Filter{"id": p.AuthorId}, nil); err != nil {
      return nil, err
    }
    return author, nil
  })
}
```

To execute the query
```graphql
query PostsWithAuthors {
  posts {
    title
    author { name }
  }
}
```
Thunder will execute `SELECT * FROM posts` and, if that returns three posts
with author IDs `10`, `20`, and `31`, a follow-up query `SELECT * FROM
authors WHERE id IN (10, 20, 31)`.

## Built-in graphiql

To get started quickly without wrangling any JavaScript, Thunder comes with
a built-in `graphiql` client as an HTTP handler. To use it, simply expose
with Go's built-in HTTP server.

```go
// Expose schema and graphiql.
http.Handle("/graphql", graphql.Handler(schema))
http.Handle("/graphiql/", http.StripPrefix("/graphiql/", graphiql.Handler()))
http.ListenAndServe(":3030", nil)
```

## Split schema building for large graphql servers

A large GraphQL server might have many resolvers on some shared types. To
keep packages reasonably-sized, Thunder's schema builder supports extending a
schema. For example, if you have a `User` type with a resolver `photos`
implemented by your `photos` package, and resolver `events` implemented by
your `calendar` package, those packages can independently register their
resolvers:

```go
package common

type User struct {}


package photos

type PhotosServer {}

func (s *PhotosServer) registerUser(schema *schemabuilder.Schema) {
  object := schema.Object("User", common.User{})
  object.FieldFunc("photos", s.fetchUserPhotos)
}


package events

type EventsServer {}

func (s *EventsServer) registerUser(schema *schemabuilder.Schema) {
  object := schema.Object("User", common.User{})
  object.FieldFunc("events", s.fetchUserEvents)
}
```

# Getting started

> First, a fair warning. The Thunder library is still a little bit tricky to use
> outside of Samsara. The examples above and below work, but eg. the `npm` client
> still requires some wrangling.

## A minimal complete server

The program below is a fully-functional graphql server written using Thunder. It
does not use `sqlgen`, `livesql`, or batching, but does include a live-updating
resolver.

```go
package main

import (
  "context"
  "net/http"
  "time"

  "github.com/samsarahq/thunder/graphql"
  "github.com/samsarahq/thunder/graphql/graphiql"
  "github.com/samsarahq/thunder/graphql/introspection"
  "github.com/samsarahq/thunder/graphql/schemabuilder"
  "github.com/samsarahq/thunder/reactive"
)

type post struct {
  Title     string
  Body      string
  CreatedAt time.Time
}

// server is our graphql server.
type server struct {
  posts []post
}

// registerQuery registers the root query type.
func (s *server) registerQuery(schema *schemabuilder.Schema) {
  obj := schema.Query()

  obj.FieldFunc("posts", func() []post {
    return s.posts
  })
}

// registerMutation registers the root mutation type.
func (s *server) registerMutation(schema *schemabuilder.Schema) {
  obj := schema.Mutation()
  obj.FieldFunc("echo", func(args struct{ Message string }) string {
    return args.Message
  })
}

// registerPost registers the post type.
func (s *server) registerPost(schema *schemabuilder.Schema) {
  obj := schema.Object("Post", post{})
  obj.FieldFunc("age", func(ctx context.Context, p *post) string {
    reactive.InvalidateAfter(ctx, 5*time.Second)
    return time.Since(p.CreatedAt).String()
  })
}

// schema builds the graphql schema.
func (s *server) schema() *graphql.Schema {
  builder := schemabuilder.NewSchema()
  s.registerQuery(builder)
  s.registerMutation(builder)
  s.registerPost(builder)
  return builder.MustBuild()
}

func main() {
  // Instantiate a server, build a server, and serve the schema on port 3030.
  server := &server{
    posts: []post{
      {Title: "first post!", Body: "I was here first!", CreatedAt: time.Now()},
      {Title: "graphql", Body: "did you hear about Thunder?", CreatedAt: time.Now()},
    },
  }

  schema := server.schema()
  introspection.AddIntrospectionToSchema(schema)

  // Expose schema and graphiql.
  http.Handle("/graphql", graphql.Handler(schema))
  http.Handle("/graphiql/", http.StripPrefix("/graphiql/", graphiql.Handler()))
  http.ListenAndServe(":3030", nil)
}
```

## Using Thunder without Websockets (POST requests)

For use with non-live clients (e.g. [Relay](https://facebook.github.io/relay/), [Apollo](https://www.apollographql.com/client/)) thunder provides an HTTP handler that can serve
POST requests, instead of having the client connect over a websocket. In this mode, thunder
does not provide live query updates.

In the above example, the `main` function would be changed to look like:

```go
func main() {
  // Instantiate a server, build a server, and serve the schema on port 3030.
  server := &server{
    posts: []post{
      {Title: "first post!", Body: "I was here first!", CreatedAt: time.Now()},
      {Title: "graphql", Body: "did you hear about Thunder?", CreatedAt: time.Now()},
    },
  }

  schema := server.schema()
  introspection.AddIntrospectionToSchema(schema)

  // Expose GraphQL POST endpoint.
  http.Handle("/graphql", graphql.HTTPHandler(schema))
  http.ListenAndServe(":3030", nil)
}
```

## Emitting a schema.json

Thunder can emit a GraphQL introspection query schema useful for compatibility with
other GraphQL tooling. Alongside code from the above example, here is a small program
for registering our schema and writing the JSON output to stdout.

```go
// schema_generator.go

func main() {
  // Instantiate a server and run the introspection query on it.
  server := &server{...}

  builderSchema := schemabuilder.NewSchema()
  server.registerQuery(builderSchema)
  server.registerMutation(builderSchema)
  // ...

  valueJSON, err := introspection.ComputeSchemaJSON(*builderSchema)
  if err != nil {
    panic(err)
  }

  fmt.Print(string(valueJSON))
}
```

This program can then be run to generate `schema.json`:
```bash
$ go run schema_generator.go > schema.json
```

## Code organization

The source code in this repository is organized as follows:
- The example/ directory contains a basic Thunder application.
- The graphql/ directory contains Thunder's graphql parser and executor.
- The reactive/ directory contains Thunder's core dependency-tracking and
  live-update mechanism.
- The batch/ directory contains Thunder's batching package.
- The diff/ and merge/ directories contain Thunder's JSON diffing library
  used for live queries.
- The livesql/ directory contains a Thunder driver for MySQL.
- The sqlgen/ directory contains a lightweight SQL query generator used by
  livesql/.

# Status

 Thunder has proven itself in production use at Samsara for close to two
 years. This repository is still under development, and there will be some
 breaking changes to the API but they should be manageable. If you're
 adventurous, please give it a try.
