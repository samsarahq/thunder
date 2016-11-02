package graphql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/samsarahq/thunder/reactive"
)

const (
	MaxSubscriptions    = 200
	MaxQueryParallelism = 50
	MinRerunInterval    = 1 * time.Second
)

type MakeCtxFunc func(context.Context) context.Context
type ReportErrorFunc func(err error, tags map[string]string)

type conn struct {
	writeMu sync.Mutex
	socket  *websocket.Conn

	schema      *Schema
	makeCtx     MakeCtxFunc
	reportError ReportErrorFunc

	url string

	mu            sync.Mutex
	subscriptions map[string]*reactive.Rerunner
}

type inEnvelope struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message"`
}

type outEnvelope struct {
	ID      string      `json:"id,omitempty"`
	Type    string      `json:"type"`
	Message interface{} `json:"message,omitempty"`
}

type subscribeMessage struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

type mutateMessage struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

type SanitizedError interface {
	SanitizedError() string
}

type SafeError struct {
	message string
}

func (e SafeError) Error() string {
	return e.message
}

func (e SafeError) SanitizedError() string {
	return e.message
}

func NewSafeError(format string, a ...interface{}) error {
	return SafeError{message: fmt.Sprintf(format, a...)}
}

func sanitizeError(err error) string {
	if sanitized, ok := err.(SanitizedError); ok {
		return sanitized.SanitizedError()
	}
	return "Internal server error"
}

func isCloseError(err error) bool {
	_, ok := err.(*websocket.CloseError)
	return ok || err == websocket.ErrCloseSent
}

func (c *conn) writeOrClose(id string, typ string, message interface{}) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if err := c.socket.WriteJSON(outEnvelope{
		ID:      id,
		Type:    typ,
		Message: message,
	}); err != nil {
		if !isCloseError(err) {
			c.socket.Close()
			log.Printf("socket.WriteJSON: %s\n", err)
		}
	}
}

func mustMarshalJson(v interface{}) string {
	bytes, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(bytes)
}

func (c *conn) handleSubscribe(id string, subscribe *subscribeMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.subscriptions[id]; ok {
		return NewSafeError("duplicate subscription")
	}

	if len(c.subscriptions)+1 > MaxSubscriptions {
		return NewSafeError("too many subscriptions")
	}

	selectionSet, err := Parse(subscribe.Query, subscribe.Variables)
	if err != nil {
		return err
	}
	if err := PrepareQuery(c.schema.Query, selectionSet); err != nil {
		return err
	}

	var previous interface{}

	e := Executor{
		MaxConcurrency: MaxQueryParallelism,
	}

	c.subscriptions[id] = reactive.NewRerunner(context.Background(), func(ctx context.Context) (interface{}, error) {
		ctx = c.makeCtx(ctx)
		current, err := e.Execute(ctx, c.schema.Query, nil, selectionSet)
		if err != nil {
			c.writeOrClose(id, "error", sanitizeError(err))
			go c.closeSubscription(id)
			if _, ok := err.(SanitizedError); !ok {
				c.reportError(err, map[string]string{"url": c.url, "query": subscribe.Query, "queryVariables": mustMarshalJson(subscribe.Variables)})
			}
			return nil, err
		}

		delta, diff := Diff(previous, current)
		previous = current

		if diff {
			c.writeOrClose(id, "update", PrepareForMarshal(delta))
		}

		return nil, nil
	}, MinRerunInterval)

	return nil
}

func (c *conn) handleMutate(id string, mutate *mutateMessage) error {
	// TODO: deduplicate code
	c.mu.Lock()
	defer c.mu.Unlock()

	selectionSet, err := Parse(mutate.Query, mutate.Variables)
	if err != nil {
		return err
	}
	if err := PrepareQuery(c.schema.Mutation, selectionSet); err != nil {
		return err
	}

	e := Executor{
		MaxConcurrency: MaxQueryParallelism,
	}

	c.subscriptions[id] = reactive.NewRerunner(context.Background(), func(ctx context.Context) (interface{}, error) {
		ctx = c.makeCtx(context.Background())
		current, err := e.Execute(ctx, c.schema.Mutation, c.schema.Mutation, selectionSet)
		if err != nil {
			c.writeOrClose(id, "error", sanitizeError(err))
			go c.closeSubscription(id)
			if _, ok := err.(SanitizedError); !ok {
				c.reportError(err, map[string]string{"url": c.url, "query": mutate.Query, "queryVariables": mustMarshalJson(mutate.Variables)})
			}
			return nil, err
		}

		delta, _ := Diff(nil, current)
		c.writeOrClose(id, "result", PrepareForMarshal(delta))

		return nil, errors.New("stop")
	}, MinRerunInterval)

	return nil
}

func (c *conn) closeSubscription(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if runner, ok := c.subscriptions[id]; ok {
		runner.Stop()
		delete(c.subscriptions, id)
	}
}

func (c *conn) closeSubscriptions() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for id, runner := range c.subscriptions {
		runner.Stop()
		delete(c.subscriptions, id)
	}
}

func (c *conn) handle(e *inEnvelope) error {
	switch e.Type {
	case "subscribe":
		var subscribe subscribeMessage
		if err := json.Unmarshal(e.Message, &subscribe); err != nil {
			return err
		}
		return c.handleSubscribe(e.ID, &subscribe)

	case "unsubscribe":
		c.closeSubscription(e.ID)
		return nil

	case "mutate":
		var mutate mutateMessage
		if err := json.Unmarshal(e.Message, &mutate); err != nil {
			return err
		}
		return c.handleMutate(e.ID, &mutate)

	case "echo":
		c.writeOrClose(e.ID, "echo", nil)
		return nil

	case "url":
		var url string
		if err := json.Unmarshal(e.Message, &url); err != nil {
			return err
		}
		c.url = url
		return nil

	default:
		return NewSafeError("unknown message type")
	}
}

func Handler(schema *Schema) http.Handler {
	upgrader := &websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		socket, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("upgrader.Upgrade: %v", err)
			return
		}
		defer socket.Close()

		makeCtx := func(ctx context.Context) context.Context {
			return ctx
		}
		reportError := func(err error, tags map[string]string) {
			log.Println("error:%v\n%s", tags, err)
		}

		ServeJSONSocket(socket, schema, makeCtx, reportError)
	})
}

func ServeJSONSocket(socket *websocket.Conn, schema *Schema, makeCtx MakeCtxFunc, reportError ReportErrorFunc) {
	c := &conn{
		socket: socket,

		schema:      schema,
		makeCtx:     makeCtx,
		reportError: reportError,

		subscriptions: make(map[string]*reactive.Rerunner),
	}

	defer c.closeSubscriptions()

	for {
		var envelope inEnvelope
		if err := c.socket.ReadJSON(&envelope); err != nil {
			if !isCloseError(err) {
				log.Println("socket.ReadJSON:", err)
			}
			return
		}

		if err := c.handle(&envelope); err != nil {
			log.Println("c.handle:", err)
			c.writeOrClose(envelope.ID, "error", sanitizeError(err))
		}
	}
}
