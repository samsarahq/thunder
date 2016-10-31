package reactive

import (
	"context"
	"time"
)

func InvalidateAfter(ctx context.Context, d time.Duration) {
	r := NewResource()
	timer := time.AfterFunc(d, r.Invalidate)
	r.Cleanup(func() { timer.Stop() })
	AddDependency(ctx, r)
}

func InvalidateAt(ctx context.Context, t time.Time) {
	InvalidateAfter(ctx, t.Sub(time.Now()))
}
