
package federation

import (
	"context"
	"fmt"
	"time"
)

// type ServiceVersion struct {
// 	Schema introspectionQueryResult
// }

// type ServiceConfig struct {
// 	Versions map[string]ServiceVersion
// 	Client   ExecutorClient
// }

// type ProxyConfig struct {
// 	Services map[string]ServiceConfig
// }

// type S3ProxyConfigLoader struct {
// }

// type LocalFileConfigLoader struct {
// }

// type ConfigPoller interface {
// 	Poll(ctx context.Context) (*ProxyConfig, error)
// }

type Proxy struct {

}

func (p *Proxy) Poll(ctx context.Context) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			fmt.Println("done")
			return nil
		case <-ticker.C:
			fmt.Println("tick")
		}
	}
	return nil
}

