This directory contains a basic example of Thunder. The code is organized as follows:

- All server code can be found in the `main.go` file.
- The `client/` directory contains a JavaScript client.

## Dependencies

This example requires [Docker](https://docker.com), [Go](https://golang.org/),
and [Node](https://nodejs.org/).

## Running the database

To start the database, run `docker-compose -f ../ci/docker-compose.yml up` to
start a MySQL server on port 3307, properly configured for use with Thunder. 

## Running the server

To run the server, first install the server's dependencies using
`go get .`.
Then, run `go run main.go` to start the server.

## Running the client

To run the client, first install the client's dependencies using `npm install`.
Then, run `npm run start` to start the client. You can access the basic client
by going to `http://localhost:3000`. To run your own queries, access the
GraphiQL client at `http://localhost:3000/graphiql`. One example GraphQL query
to fetch all messages is as follows:
```
{
  messages {
    id
    text
  }
}
```