package graphql

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/samsarahq/thunder/batch"
	"github.com/samsarahq/thunder/diff"
	"github.com/samsarahq/thunder/reactive"
)

func HTTPHandler(schema *Schema, middlewares ...MiddlewareFunc) http.Handler {
	return &httpHandler{
		schema:      schema,
		middlewares: middlewares,
	}
}

type httpHandler struct {
	schema      *Schema
	middlewares []MiddlewareFunc
}

type httpPostBody struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

type httpResponse struct {
	Data   interface{}   `json:"data"`
	Errors []interface{} `json:"errors"`
}

func (h *httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	writeResponse := func(value interface{}, err error) {
		response := httpResponse{}
		if err != nil {
			response.Errors = []interface{}{err.Error()}
		} else {
			response.Data = diff.StripKey(value)
		}

		responseJSON, err := json.Marshal(response)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		http.Error(w, string(responseJSON), http.StatusOK)
	}

	if r.Method != "POST" {
		writeResponse(nil, errors.New("request must be a POST"))
		return
	}

	if r.Body == nil {
		writeResponse(nil, errors.New("request must include a query"))
		return
	}

	var params httpPostBody
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		writeResponse(nil, err)
		return
	}

	query, err := Parse(params.Query, params.Variables)
	if err != nil {
		writeResponse(nil, err)
		return
	}

	schema := h.schema.Query
	if query.Kind == "mutation" {
		schema = h.schema.Mutation
	}
	if err := PrepareQuery(schema, query.SelectionSet); err != nil {
		writeResponse(nil, err)
		return
	}

	var queries []*Query
	for _, s := range query.SelectionSet.Selections {
		q := &Query{
			Name:         s.Name,
			Kind:         query.Kind,
			SelectionSet: &SelectionSet{Selections: []*Selection{s}},
		}
		queries = append(queries, q)
	}

	response := NewSyncResponse()

	collectResponse := func(selection string, data interface{}, err error) {

		if err != nil {
			response.StoreErr(selection, err)
			return
		}

		if d, ok := data.(map[string]interface{}); ok {
			for key, val := range d {
				response.Store(key, val)
			}
			return
		}

		response.Store(selection, data)
	}

	finalizeResponse := func(value *syncMap, errors *syncMap) {
		response := httpResponse{
			Errors: errors.Errors(),
			Data:   diff.StripKey(value.internal),
		}

		responseJSON, err := json.Marshal(response)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		http.Error(w, string(responseJSON), http.StatusOK)
	}

	var wg sync.WaitGroup

	runners := make([]*reactive.Rerunner, len(queries))
	for i, q := range queries {
		qry := q

		e := Executor{}

		wg.Add(1)
		runner := reactive.NewRerunner(r.Context(), func(ctx context.Context) (interface{}, error) {
			defer wg.Done()

			ctx = batch.WithBatching(ctx)

			var middlewares []MiddlewareFunc
			middlewares = append(middlewares, h.middlewares...)
			middlewares = append(middlewares, func(input *ComputationInput, next MiddlewareNextFunc) *ComputationOutput {
				output := next(input)
				output.Current, output.Error = e.Execute(input.Ctx, schema, nil, input.ParsedQuery)
				return output
			})

			output := RunMiddlewares(middlewares, &ComputationInput{
				Ctx:         ctx,
				ParsedQuery: qry,
				Query:       params.Query,
				Variables:   params.Variables,
			})
			current, err := output.Current, output.Error

			if err != nil {
				if ErrorCause(err) == context.Canceled {
					return nil, err
				}

				collectResponse(qry.Name, nil, err)
				return nil, err
			}

			collectResponse(qry.Name, current, nil)
			return nil, nil
		}, DefaultMinRerunInterval)

		runners[i] = runner
	}

	wg.Wait()

	for _, runner := range runners {
		runner.Stop()
	}

	finalizeResponse(response.Data, response.Errors)

}
