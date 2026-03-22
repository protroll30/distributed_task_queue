package redis

import (
	goredis "github.com/redis/go-redis/v9"
)

// New returns a client for addr. The caller must Close the client when it is no longer needed.
func New(addr string) *goredis.Client {
	return goredis.NewClient(&goredis.Options{Addr: addr})
}
