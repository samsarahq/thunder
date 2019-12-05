package federation

import "context"

type ServiceVersion struct {
	Schema introspectionQueryResult
}

type ServiceConfig struct {
	Versions map[string]ServiceVersion
	Client   ExecutorClient
}

type ProxyConfig struct {
	Services map[string]ServiceConfig
}

type S3ProxyConfigLoader struct {
}

type LocalFileConfigLoader struct {
}

type ConfigPoller interface {
	Poll(ctx context.Context) (*ProxyConfig, error)
}

type Proxy struct {
}

func (p *Proxy) UpdateConfig(c *ProxyConfig) error {
	return nil
}
