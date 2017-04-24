package helpers

import (
	"context"

	"github.com/opentracing/opentracing-go"
)

type MockSpan struct {
	opentracing.Span
}

func (m *MockSpan) Finish()                                          {}
func (m *MockSpan) FinishWithOptions(opts opentracing.FinishOptions) {}

func MaybeStartSpanFromContext(
	ctx context.Context,
	operationName string,
	opts ...opentracing.StartSpanOption,
) (opentracing.Span, context.Context) {
	if span := opentracing.SpanFromContext(ctx); span != nil {
		span, ctx := opentracing.StartSpanFromContext(ctx, operationName, opts...)
		return span, ctx
	} else {
		// If there is no parent span, we pass back a working
		// MockSpan for the current context to call on, but not
		// report back to the underlying tracer.
		// We also do not contribute the new span to the
		// context so that downstream spans do not declare
		// themselves children of the MockSpan.
		span, _ := opentracing.StartSpanFromContext(ctx, operationName, opts...)
		return &MockSpan{Span: span}, ctx
	}
}
