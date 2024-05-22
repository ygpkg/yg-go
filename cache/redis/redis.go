package redis

import (
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/ygpkg/yg-go/cache"
)

var _ cache.Cache = (*Redis)(nil)

// Redis redis cache
type Redis struct {
	conn *redis.Pool
}

// RedisOpts redis 连接属性
type RedisOpts struct {
	Host        string `yaml:"host" json:"host"`
	Password    string `yaml:"password" json:"password"`
	Database    int    `yaml:"database" json:"database"`
	MaxIdle     int    `yaml:"max_idle" json:"max_idle"`
	MaxActive   int    `yaml:"max_active" json:"max_active"`
	IdleTimeout int32  `yaml:"idle_timeout" json:"idle_timeout"` //second
}

// NewCache 实例化
func NewCache(opts *RedisOpts) *Redis {
	pool := &redis.Pool{
		MaxActive:   opts.MaxActive,
		MaxIdle:     opts.MaxIdle,
		IdleTimeout: time.Second * time.Duration(opts.IdleTimeout),
		Dial: func() (redis.Conn, error) {
			return redis.Dial("tcp", opts.Host,
				redis.DialDatabase(opts.Database),
				redis.DialPassword(opts.Password),
			)
		},
		TestOnBorrow: func(conn redis.Conn, t time.Time) error {
			if time.Since(t) < time.Minute {
				return nil
			}
			_, err := conn.Do("PING")
			return err
		},
	}
	return &Redis{pool}
}

func NewCacheByConn(pool *redis.Pool) *Redis {
	return &Redis{pool}
}

// SetConn 设置conn
func (r *Redis) SetConn(conn *redis.Pool) {
	r.conn = conn
}

// Get 获取一个值
func (r *Redis) Get(key string, reply interface{}) error {
	conn := r.conn.Get()
	defer conn.Close()

	var data []byte
	var err error
	if data, err = redis.Bytes(conn.Do("GET", key)); err != nil {
		return err
	}
	if err = cache.Unmarshal(data, reply); err != nil {
		return err
	}

	return nil
}

// Set 设置一个值
func (r *Redis) Set(key string, val interface{}, timeout time.Duration) (err error) {
	conn := r.conn.Get()
	defer conn.Close()

	data := cache.Marshal(val)
	_, err = conn.Do("SETEX", key, int64(timeout/time.Second), data)

	return
}

// IsExist 判断key是否存在
func (r *Redis) IsExist(key string) bool {
	conn := r.conn.Get()
	defer conn.Close()

	a, _ := conn.Do("EXISTS", key)
	i := a.(int64)
	if i > 0 {
		return true
	}
	return false
}

// Delete 删除
func (r *Redis) Delete(key string) error {
	conn := r.conn.Get()
	defer conn.Close()

	if _, err := conn.Do("DEL", key); err != nil {
		return err
	}

	return nil
}
