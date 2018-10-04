package graphql

import (
	"net/http"

	"github.com/gorilla/websocket"
)

func HybridHandler(schema *Schema) http.Handler {
	handler := Handler(schema)
	fallback := HTTPHandler(schema)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if websocket.IsWebSocketUpgrade(r) {
			handler.ServeHTTP(w, r)
			return
		}

		fallback.ServeHTTP(w, r)
	})
}
