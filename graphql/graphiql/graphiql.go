package graphiql

//go:generate yarn
//go:generate yarn webpack -p
//go:generate statik -src ./dist

import (
	"net/http"

	"github.com/rakyll/statik/fs"

	_ "github.com/samsarahq/thunder/graphql/graphiql/statik"
)

func Handler() http.Handler {
	statikFS, err := fs.New()
	if err != nil {
		panic(err)
	}
	return http.FileServer(statikFS)
}
