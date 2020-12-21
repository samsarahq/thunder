package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/samsarahq/thunder/thunderpb"
	"github.com/samsarahq/thunder/federation"
	"google.golang.org/grpc"

	"github.com/samsarahq/thunder/graphql"
)



func HTTPHandler() http.Handler {
	return &httpHandler{}
}

type httpHandler struct {
	gatewayClient *federation.ExecutorClient
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
	writeResponse := func(value interface{}, err error) {
		response := httpResponse{}
		if err != nil {
			response.Errors = []string{err.Error()}
		} else {
			response.Data = value
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

	ctx := r.Context()

	if r.Method != "POST" {
		writeResponse(nil, errors.New("request must be a POST"))
		return
	}

	if r.Body == nil {
		writeResponse(nil, errors.New("request must include a query"))
		return
	}

	var request httpPostBody
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	query, err := graphql.Parse(request.Query, request.Variables)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cc, err := grpc.DialContext(ctx, "localhost:1236", grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	gatewayClient := &federation.GrpcExecutorClient{Client: thunderpb.NewExecutorClient(cc)}
	result, err := gatewayClient.Execute(ctx, &federation.QueryRequest{Query: query})
	if err != nil {
		writeResponse(nil, err)
	}
	var res interface{}
	if result != nil {
		if err := json.Unmarshal(result.Result, &res); err != nil {
			writeResponse(nil, err)
		}
	}
	writeResponse(res, nil)

}



func main() {
	http.Handle("/", http.FileServer(http.Dir(".")))
	http.Handle("/graphql", HTTPHandler())
	http.ListenAndServe(":3030", nil)
}
