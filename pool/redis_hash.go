package pool

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ygpkg/yg-go/config"
)

var _ Pool = (*RedisHashPool)(nil)

// RedisPool 实现 Pool 接口
type RedisHashPool struct {
	cli     *redis.Client
	ctx     context.Context
	svrname string                   // 服务名
	sgroup  string                   // 组名
	conf    config.ServicePoolConfig //服务配置
	expire  time.Duration            // 过期时间
}

// NewRedisPool 创建一个新的 服务池 每次新建一个服务池时，会清空原有服务池
func NewRedisHashPool(ctx context.Context, cli *redis.Client, expire time.Duration, svrname, sgroup string, conf config.ServicePoolConfig) *RedisHashPool {
	if ctx == nil {
		ctx = context.Background()
	}
	hashKey := svrname + ":" + sgroup
	hashFields := map[string]interface{}{}
	for _, v := range conf.Services {
		for i := 1; i <= v.Cap; i++ {
			key := strconv.Itoa(i) + "_" + v.Name
			hashFields[key] = 0 // 默认值
		}
	}
	err := cli.HSet(ctx, hashKey, hashFields).Err()
	if err != nil {
		panic(err)
	}

	return &RedisHashPool{
		ctx:     ctx,
		cli:     cli,
		sgroup:  sgroup,
		svrname: svrname,
		conf:    conf,
		expire:  expire,
	}
}

// Acquire 从资源池中获取一个资源, 返回值为interface
// 不需要key
func (rp *RedisHashPool) Acquire() (interface{}, error) {
	res, err := rp.cli.HGetAll(rp.ctx, rp.svrname+":"+rp.sgroup).Result()
	if err != nil {
		return nil, err
	}
	// 获取时间变量
	now := time.Now()
	futureTime := now.Add(rp.expire)
	nowNano := now.UnixNano()
	futureNano := futureTime.UnixNano()
	for k, v := range res {
		intValue, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, err
		}
		if nowNano > intValue {
			hashFields := map[string]interface{}{}
			hashFields[k] = futureNano // 后延过期时间
			err := rp.cli.HSet(rp.ctx, rp.svrname+":"+rp.sgroup, hashFields).Err()
			if err != nil {
				return nil, err
			}
			return k, nil
		}
	}
	return "", fmt.Errorf("no service" + rp.svrname + ":" + rp.sgroup)
}

// Release 释放一个资源到资源池, v为string
func (rp *RedisHashPool) Release(v interface{}) error {
	key := v.(string)
	exists, err := rp.cli.HExists(rp.ctx, rp.svrname+":"+rp.sgroup, key).Result()
	if err != nil {
		return err
	}
	if exists {
		hashFields := map[string]interface{}{}
		hashFields[key] = 0 // 归还时设为0
		err := rp.cli.HSet(rp.ctx, rp.svrname+":"+rp.sgroup, hashFields).Err()
		if err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("no key %s", key)
}

// AcquireDecode 从资源池中获取一个资源, 并json解析到v
func (rp *RedisHashPool) AcquireDecode(v interface{}) error {
	k, err := rp.Acquire()
	if err != nil {
		return err
	}
	key := k.(string)
	// 找到第一个 _ 的位置
	index := strings.Index(key, "_")
	if index == -1 {
		return fmt.Errorf("error key %s", key)
	}
	// 分成两个字符串
	jsonkey := key[:index]
	value := key[index+1:]
	// 组成字典
	result := map[string]string{
		jsonkey: value,
	}
	// 将字典转换为 JSON 字符串
	jsonData, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("JSON encoding failed:" + err.Error())
	}
	return json.Unmarshal(jsonData, v)
}

// ReleaseEncode 释放一个资源到资源池, 并json编码
func (rp *RedisHashPool) ReleaseEncode(v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var m map[string]interface{}
	err = json.Unmarshal(data, &m)
	if err != nil {
		return err
	}
	key := ""
	for k, v := range m {
		key = k + "_" + v.(string)
	}
	rp.Release(key)
	return nil
}

// AcquireString 从资源池中获取一个资源, 返回值为string
func (rp *RedisHashPool) AcquireString() (string, error) {
	k, err := rp.Acquire()
	if err != nil {
		return "", err
	}
	key := k.(string)
	return key, nil
}

// ReleaseString 释放一个资源到资源池, v为string
func (rp *RedisHashPool) ReleaseString(v string) error {
	return rp.Release(v)
}

// AcquireKeyIndex 从资源池中获取一个资源
// 返回值为整个key，index_url
func (rp *RedisHashPool) AcquireKeyIndex(service string) (string, error) {
	res, err := rp.cli.HGetAll(rp.ctx, rp.svrname+":"+rp.sgroup).Result()
	if err != nil {
		return "", err
	}
	// 获取时间变量
	now := time.Now()
	futureTime := now.Add(rp.expire)
	nowNano := now.UnixNano()
	futureNano := futureTime.UnixNano()
	for k, v := range res {
		// 找到第一个 _ 的位置
		index := strings.Index(k, "_")
		result := ""
		if index != -1 {
			// 获取第一个 _ 之后的所有字符
			result = strings.TrimPrefix(k[index:], "_")
		} else {
			continue
		}
		if result == service {
			// 匹配业务成功
			intValue, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return "", err
			}
			if nowNano > intValue {
				// 有空闲可以持有
				// 设置新的过期时间
				hashFields := map[string]interface{}{}
				hashFields[k] = futureNano // 默认值
				err := rp.cli.HSet(rp.ctx, rp.svrname+":"+rp.sgroup, hashFields).Err()
				if err != nil {
					return "", err
				}
				return k, nil
			}
		}
	}
	return "", fmt.Errorf("no service %s", service)
}

// ReleaseKeyIndex 释放一个资源到资源池, key:index_url
func (rp *RedisHashPool) ReleaseKeyIndex(key string) error {
	exists, err := rp.cli.HExists(rp.ctx, rp.svrname+":"+rp.sgroup, key).Result()
	if err != nil {
		return err
	}
	if exists {
		hashFields := map[string]interface{}{}
		hashFields[key] = 0 // 归还时设为0
		err := rp.cli.HSet(rp.ctx, rp.svrname+":"+rp.sgroup, hashFields).Err()
		if err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("no key %s", key)
}

// Clear 清空资源池
func (rp *RedisHashPool) Clear() {
	rp.cli.HDel(rp.ctx, rp.svrname+":"+rp.sgroup)
}
