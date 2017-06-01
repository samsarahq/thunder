package opentracingkit

import (
	"context"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"
)

var noopTracer = &opentracing.NoopTracer{}

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
		// NoopSpan for the current context to call on, but not
		// report back to the underlying tracer.
		// We also do not contribute the new span to the
		// context so that downstream spans do not declare
		// themselves children of the NoopSpan.
		span := noopTracer.StartSpan(operationName)
		return span, ctx
	}
}

func LogError(span opentracing.Span, err error) {
	ext.Error.Set(span, true)
	span.LogFields(log.Error(err))
}
