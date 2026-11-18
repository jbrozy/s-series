// Package cache wraps Redis: sessions, job queue, rate limits and the
// model answer cache.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/gomodule/redigo/redis"
)

type Redis struct {
	pool *redis.Pool
}

func New(addr, password string, db int) *Redis {
	return &Redis{pool: &redis.Pool{
		MaxIdle:     10,
		MaxActive:   50,
		IdleTimeout: 4 * time.Minute,
		Wait:        true,
		Dial: func() (redis.Conn, error) {
			opts := []redis.DialOption{redis.DialDatabase(db), redis.DialConnectTimeout(5 * time.Second)}
			if password != "" {
				opts = append(opts, redis.DialPassword(password))
			}
			return redis.Dial("tcp", addr, opts...)
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			if time.Since(t) < time.Minute {
				return nil
			}
			_, err := c.Do("PING")
			return err
		},
	}}
}

func (r *Redis) Ping(ctx context.Context) error {
	c, err := r.pool.GetContext(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	_, err = c.Do("PING")
	return err
}

// WaitReady retries until Redis answers.
func (r *Redis) WaitReady(ctx context.Context) error {
	var err error
	for i := 0; i < 30; i++ {
		if err = r.Ping(ctx); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return err
}

func (r *Redis) do(cmd string, args ...any) (any, error) {
	c := r.pool.Get()
	defer c.Close()
	return c.Do(cmd, args...)
}

func (r *Redis) SetEX(key string, ttl time.Duration, val string) error {
	_, err := r.do("SET", key, val, "EX", int(ttl.Seconds()))
	return err
}

func (r *Redis) Get(key string) (string, bool) {
	s, err := redis.String(r.do("GET", key))
	if err != nil {
		return "", false
	}
	return s, true
}

func (r *Redis) Del(key string) error {
	_, err := r.do("DEL", key)
	return err
}

func (r *Redis) Expire(key string, ttl time.Duration) {
	_, _ = r.do("EXPIRE", key, int(ttl.Seconds()))
}

// Incr increments a counter with TTL (rate limiting).
func (r *Redis) Incr(key string, ttl time.Duration) (int, error) {
	n, err := redis.Int(r.do("INCR", key))
	if err != nil {
		return 0, err
	}
	if n == 1 {
		r.Expire(key, ttl)
	}
	return n, nil
}

const queueKey = "scheck:jobs"

// Job is a unit of background work.
type Job struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Arg  string `json:"arg,omitempty"`
}

func (r *Redis) Enqueue(j Job) error {
	b, _ := json.Marshal(j)
	_, err := r.do("LPUSH", queueKey, string(b))
	return err
}

// Dequeue blocks up to timeout for the next job.
func (r *Redis) Dequeue(timeout time.Duration) (*Job, error) {
	c := r.pool.Get()
	defer c.Close()
	vals, err := redis.Strings(c.Do("BRPOP", queueKey, int(timeout.Seconds())))
	if errors.Is(err, redis.ErrNil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var j Job
	if err := json.Unmarshal([]byte(vals[1]), &j); err != nil {
		return nil, err
	}
	return &j, nil
}

func (r *Redis) QueueLength() int {
	n, _ := redis.Int(r.do("LLEN", queueKey))
	return n
}

// LLMCache adapts Redis to analysis.Cache.
type LLMCache struct {
	R   *Redis
	TTL time.Duration
}

func (l LLMCache) Get(key string) (string, bool) { return l.R.Get("scheck:llm:" + key) }
func (l LLMCache) Set(key, value string)         { _ = l.R.SetEX("scheck:llm:"+key, l.TTL, value) }
