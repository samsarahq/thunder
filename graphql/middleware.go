package graphql

import "context"

type ComputationInput struct {
	Id          string
	Query       string
	ParsedQuery *Query
	Variables   map[string]interface{}
	Ctx         context.Context
	Previous    interface{}
}

type ComputationOutput struct {
	Metadata map[string]interface{}
	Current  interface{}
	Error    error
}

type MiddlewareFunc func(input *ComputationInput, output *ComputationOutput, next NextFunc)
type NextFunc func(input *ComputationInput, output *ComputationOutput)

func _runMiddlewares(index int, middlewares []MiddlewareFunc, input *ComputationInput, output *ComputationOutput) {
	if index >= len(middlewares) {
		return
	}

	middleware := middlewares[index]
	middleware(input, output, func(input *ComputationInput, output *ComputationOutput) {
		_runMiddlewares(index+1, middlewares, input, output)
	})
}

func runMiddlewares(middlewares []MiddlewareFunc, input *ComputationInput, output *ComputationOutput) {
	_runMiddlewares(0, middlewares, input, output)
}
