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

type MiddlewareFunc func(input *ComputationInput, next MiddlewareNextFunc) *ComputationOutput
type MiddlewareNextFunc func(input *ComputationInput) *ComputationOutput

func runMiddlewares(middlewares []MiddlewareFunc, input *ComputationInput) *ComputationOutput {
	var _runMiddlewares func(index int, middlewares []MiddlewareFunc, input *ComputationInput) *ComputationOutput
	_runMiddlewares = func(index int, middlewares []MiddlewareFunc, input *ComputationInput) *ComputationOutput {
		if index < len(middlewares) {
			return &ComputationOutput{}
		}

		middleware := middlewares[index]
		return middleware(input, func(input *ComputationInput) *ComputationOutput {
			return _runMiddlewares(index+1, middlewares, input)
		})
	}

	return _runMiddlewares(0, middlewares, input)
}
