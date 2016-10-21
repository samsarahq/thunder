package main

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"samsaradev.io/thunder"

	"github.com/garyburd/redigo/redis"
)

var redisPool = &redis.Pool{
	MaxIdle:     100,
	MaxActive:   100,
	Wait:        true,
	IdleTimeout: 240 * time.Second,
	Dial: func() (redis.Conn, error) {
		c, err := redis.Dial("tcp", ":6379")
		if err != nil {
			return nil, err
		}
		return c, err
	},
	TestOnBorrow: func(c redis.Conn, t time.Time) error {
		_, err := c.Do("PING")
		return err
	},
}

/*
func SetupRedis() {
	rl := &RedisLog{
		Resources: make(map[string]map[*thunder.Resource]struct{}),
	}
	go rl.run()
	time.Sleep(100 * time.Millisecond)

	Redis = &LiveRedis{
		Binlog: rl,
	}
}
*/

type RedisLog struct {
	mu        sync.Mutex
	Resources map[string]*thunder.Resource
}

func (rl *RedisLog) process(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// invalidate resource specified by key
	if r, ok := rl.Resources[key]; ok {
		r.Strobe()
	}
}

func (rl *RedisLog) DependOn(ctx context.Context, key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if r, ok := rl.Resources[key]; ok {
		thunder.AddDependency(ctx, r)
		return
	}

	r := thunder.NewResource()
	r.Cleanup(func() {
		rl.mu.Lock()
		defer rl.mu.Unlock()
		delete(rl.Resources, key)
	})

	rl.Resources[key] = r
	thunder.AddDependency(ctx, r)
}

func (rl *RedisLog) run() {
	psc := redis.PubSubConn{redisPool.Get()}
	defer psc.Close()

	prefix := "__keyspace@0__:"
	if err := psc.PSubscribe(prefix + "*"); err != nil {
		log.Fatal(err)
	}

	for {
		switch n := psc.Receive().(type) {
		case redis.Message:
		case redis.PMessage:
			if strings.HasPrefix(n.Channel, prefix) {
				key := strings.TrimPrefix(n.Channel, prefix)
				rl.process(key)
			}
		case redis.Subscription:
		case error:
			log.Fatal(n)
		}
	}
}

type LiveRedis struct {
	Binlog *RedisLog
}

func (r *LiveRedis) LPush(values ...interface{}) error {
	c := redisPool.Get()
	defer c.Close()

	_, err := c.Do("LPUSH", values...)
	return err
}

type lRangeCacheKey struct {
	key         string
	first, last int
}

func (r *LiveRedis) LRange(ctx context.Context, key string, first, last int) ([]string, error) {
	cacheKey := lRangeCacheKey{key: key, first: first, last: last}
	results, err := thunder.Cache(ctx, cacheKey, func(ctx context.Context) (interface{}, error) {
		c := redisPool.Get()
		defer c.Close()

		r.Binlog.DependOn(ctx, key)
		return redis.Strings(c.Do("LRANGE", key, first, last))
	})
	if err != nil {
		return nil, err
	}
	return results.([]string), nil
}
