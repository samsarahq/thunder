package federation
import (
	"context"

	"github.com/samsarahq/thunder/graphql"
)

type ExecutorClient interface {
	Execute(ctx context.Context, req *graphql.Query) ([]byte, error)
}
