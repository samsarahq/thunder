package federation

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/samsarahq/thunder/thunderpb"
	"google.golang.org/grpc"

	"github.com/samsarahq/thunder/graphql"
)

func HTTPHandler() http.Handler {
	return &httpHandler{}
}

type httpHandler struct {
	// executor *Executor
	// gateway       *Gateway
	gatewayClient *GatewayExecutorClient
}

type httpPostBody struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

type httpResponse struct {
	Data   interface{} `json:"data"`
	Errors []string    `json:"errors"`
}

// XXX: share code with graphql/http?
// XXX: middleware

func (h *httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	writeResponse := func(value interface{}, err error) {
		// XXX: error codes?
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

	// XXX: GET for requests?
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
	gatewayClient := &GrpcExecutorClient{Client: thunderpb.NewExecutorClient(cc)}
	result, err := gatewayClient.Execute(ctx, query)
	if err != nil {
		log.Fatal(err)
	}
	var res interface{}
	if err := json.Unmarshal(result, &res); err != nil {
		log.Fatal(err)
	}
	writeResponse(res, nil)

	// writeResponse(res, nil)
}
