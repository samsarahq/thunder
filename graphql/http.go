package graphql

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"fmt"

	"github.com/samsarahq/thunder/batch"
	"github.com/samsarahq/thunder/reactive"
)

func HTTPHandler(schema *Schema, middlewares ...MiddlewareFunc) http.Handler {
	return HTTPHandlerWithExecutor(schema, (NewExecutor(NewImmediateGoroutineScheduler())), middlewares...)
}

func HTTPHandlerWithExecutor(schema *Schema, executor ExecutorRunner, middlewares ...MiddlewareFunc) http.Handler {
	return &httpHandler{
		schema:      schema,
		middlewares: middlewares,
		executor:    executor,
	}
}

type httpHandler struct {
	schema      *Schema
	middlewares []MiddlewareFunc
	executor    ExecutorRunner
}

type httpPostBody struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

type httpResponse struct {
	Data   interface{} `json:"data"`
	Errors []string    `json:"errors"`
}

func (h *httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	writeResponse := func(value interface{}, partialErrors interface{}, err error) {
		response := httpResponse{}
		if err != nil {
			response.Errors = []string{err.Error()}
		} else {
			response.Data = value

			partialErrors := partialErrors.([]error)
			errorStrings := make([]string, 0, len(partialErrors))
			for _, partialErr := range partialErrors{
				errorStrings = append(errorStrings,partialErr.Error() )
			}
			response.Errors = errorStrings
		}

		responseJSON, err := json.Marshal(response)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json")
		}
		w.Write(responseJSON)
	}

	if r.Method != "POST" {
		writeResponse(nil, nil, errors.New("request must be a POST"))
		return
	}

	if r.Body == nil {
		writeResponse(nil, nil, errors.New("request must include a query"))
		return
	}

	var params httpPostBody
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		writeResponse(nil, nil,  err)
		return
	}

	query, err := Parse(params.Query, params.Variables)
	if err != nil {
		writeResponse(nil, nil, err)
		return
	}

	schema := h.schema.Query
	if query.Kind == "mutation" {
		schema = h.schema.Mutation
	}
	if err := PrepareQuery(r.Context(), schema, query.SelectionSet); err != nil {
		writeResponse(nil, nil, err)
		return
	}

	var wg sync.WaitGroup
	e := h.executor

	wg.Add(1)
	runner := reactive.NewRerunner(r.Context(), func(ctx context.Context) (interface{}, error) {
		defer wg.Done()

		ctx = batch.WithBatching(ctx)

		var middlewares []MiddlewareFunc
		middlewares = append(middlewares, h.middlewares...)
		middlewares = append(middlewares, func(input *ComputationInput, next MiddlewareNextFunc) *ComputationOutput {
			output := next(input)
			// output.Current, _, output.Error = e.Execute(input.Ctx, schema, nil, input.ParsedQuery)
			executeErrors := []error{}
			output.Current, executeErrors, output.Error = e.ExecuteWithPartialFailures(input.Ctx, schema, nil, input.ParsedQuery)
			output.Metadata["errors"] = executeErrors
			fmt.Println("YPOOOO4", output.Current)
			return output
		})

		output := RunMiddlewares(middlewares, &ComputationInput{
			Ctx:         ctx,
			ParsedQuery: query,
			Query:       params.Query,
			Variables:   params.Variables,
		})
		current, partialErrors, err := output.Current, output.Metadata["errors"], output.Error

		if err != nil {
			if ErrorCause(err) == context.Canceled {
				return nil, err
			}

			writeResponse(nil, nil, err)
			return nil, err
		}

		writeResponse(current, partialErrors,  nil)
		return nil, nil
	}, DefaultMinRerunInterval, false)

	wg.Wait()
	runner.Stop()
}
