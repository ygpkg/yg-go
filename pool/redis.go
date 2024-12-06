package pool

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/logs"
)

var _ Pool = (*RedisPool)(nil)

// RedisPool 实现 Pool 接口
type RedisPool struct {
	cli  *redis.Client
	ctx  context.Context
	key  string                   // 服务名
	conf config.ServicePoolConfig //服务配置
}

// NewRedisPool 创建一个新的 服务池 每次新建一个服务池时，会清空原有服务池
func NewRedisPool(ctx context.Context, cli *redis.Client, key string, conf config.ServicePoolConfig) *RedisPool {
	if ctx == nil {
		ctx = context.Background()
	}

	// 添加成员到 ZSET
	zsetMembers := []redis.Z{}
	for _, v := range conf.Services {
		for i := 1; i <= v.Cap; i++ {
			member := strconv.Itoa(i) + "_" + v.Name
			zsetMembers = append(zsetMembers, redis.Z{
				Score:  0, // 默认分数
				Member: member,
			})
		}
	}

	// 将成员添加到 ZSET
	err := cli.ZAdd(ctx, key, zsetMembers...).Err()
	if err != nil {
		logs.Errorf("zadd error", err)
		panic(err)
	}
	logs.Infof("new redis pool success")

	return &RedisPool{
		ctx:  ctx,
		cli:  cli,
		key:  key,
		conf: conf,
	}
}

// Acquire 从资源池中获取一个资源, 返回值为redis.z
// 不需要key
func (rp *RedisPool) Acquire() (interface{}, error) {
	// 获取时间变量
	now := time.Now()
	futureTime := now.Add(rp.conf.Expire)
	nowNano := float64(now.UnixNano())
	futureNano := float64(futureTime.UnixNano())
	// 拿一个最小的
	members, err := rp.cli.ZPopMin(rp.ctx, rp.key, 1).Result()
	if err != nil {
		logs.Warnf("no service member %s", err.Error())
		return nil, err
	}

	if len(members) == 0 {
		logs.Warnf("no service member %s", rp.key)
		return nil, fmt.Errorf("no service %s", rp.key)
	}

	minScore := members[0].Score
	if minScore > nowNano {
		// 没有空闲的放回去
		err = rp.cli.ZAdd(rp.ctx, rp.key, members[0]).Err()
		if err != nil {
			return nil, err
		}
		logs.Warnf("no service member %s", rp.key)
		return "", fmt.Errorf("no service %s", rp.key)
	}
	members[0].Score = futureNano
	// 后延时间
	err = rp.cli.ZAdd(rp.ctx, rp.key, members[0]).Err()
	if err != nil {
		return nil, err
	}
	logs.Infof("acquire success %s", members[0].Member.(string))
	return members[0], nil
}

// Release 释放一个资源到资源池, v为redis.z
func (rp *RedisPool) Release(v interface{}) error {
	rz := v.(redis.Z)
	// 使用 WATCH 来监视键
	for {
		err := rp.cli.Watch(rp.ctx, func(tx *redis.Tx) error {
			// 获取成员的分数
			score, err := tx.ZScore(rp.ctx, rp.key, rz.Member.(string)).Result()
			if err == redis.Nil {
				logs.Warnf("member does not exist.%s", err.Error())
				return fmt.Errorf("member does not exist.%s", err.Error())
			} else if err != nil {
				logs.Warnf("ZScore error.%s", err.Error())
				return err
			}

			// 检查分数是否匹配
			if score == rz.Score {
				// 相等
				// 开启事务
				_, err := tx.Pipelined(rp.ctx, func(pipe redis.Pipeliner) error {
					// 归还，设置默认值
					rz.Score = 0
					return pipe.ZAdd(rp.ctx, rp.key, rz).Err()
				})
				if err != nil {
					return err
				}
				return nil
			} else {
				// 不相等没有归还，但是也相当于归还成功
				// return fmt.Errorf("score not match")
				// 日志
				return nil
			}

		}, rp.key)
		if err == redis.TxFailedErr {
			logs.Warnf("Someone modified the data and tried again: %s", err.Error())
			// 有人修改数据重试
			continue
		}
		if err != nil {
			logs.Errorf("watch error %s", err.Error())
			return err
		}
		logs.Infof("release success %s", rz.Member.(string))
		return nil
	}
}

// AcquireDecode 从资源池中获取一个资源, 并json解析到v
func (rp *RedisPool) AcquireDecode(v interface{}) error {
	rz, err := rp.Acquire()
	if err != nil {
		return err
	}
	// 将字典转换为 JSON 字符串
	jsonData, err := json.Marshal(rz)
	if err != nil {
		return fmt.Errorf("JSON encoding failed:" + err.Error())
	}
	return json.Unmarshal(jsonData, &v)
}

// ReleaseEncode 释放一个资源到资源池, 并json编码
func (rp *RedisPool) ReleaseEncode(v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var rz redis.Z
	err = json.Unmarshal(data, &rz)
	if err != nil {
		return err
	}
	rp.Release(rz)
	return nil
}

// AcquireString 从资源池中获取一个资源, 返回值为string
func (rp *RedisPool) AcquireString() (string, error) {
	rz, err := rp.Acquire()
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(rz)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ReleaseString 释放一个资源到资源池, v为string
func (rp *RedisPool) ReleaseString(v string) error {
	var rz redis.Z
	err := json.Unmarshal([]byte(v), &rz)
	if err != nil {
		return err
	}
	return rp.Release(rz)
}

// Clear 清空资源池
func (rp *RedisPool) Clear() {
	rp.cli.ZRemRangeByRank(rp.ctx, rp.key, 0, -1)
}

// RefreshConfigs 重新加载配置, oldConf为旧配置, newConf为新配置
func (rp *RedisPool) RefreshConfigs(newConf config.ServicePoolConfig) error {
	// 比较服务列表
	oldServices := make(map[string]config.ServiceInfo)
	newServices := make(map[string]config.ServiceInfo)

	for _, service := range rp.conf.Services {
		oldServices[service.Name] = service
	}

	for _, service := range newConf.Services {
		newServices[service.Name] = service
	}
	for {
		err := rp.cli.Watch(rp.ctx, func(tx *redis.Tx) error {
			// 找出新增的服务
			for name, newService := range newServices {
				if _, exists := oldServices[name]; !exists {
					// added = append(added, newService)
					zsetMembers := []redis.Z{}
					for i := 1; i <= newService.Cap; i++ {
						member := strconv.Itoa(i) + "_" + newService.Name
						zsetMembers = append(zsetMembers, redis.Z{
							Score:  0, // 默认分数
							Member: member,
						})
					}
					err := tx.ZAdd(rp.ctx, rp.key, zsetMembers...).Err()
					if err != nil {
						return err
					}
				}
			}

			// 找出被删除的服务
			for name, delService := range oldServices {
				if _, exists := newServices[name]; !exists {
					// removed = append(removed, delService)
					for i := 1; i <= delService.Cap; i++ {
						err := tx.ZRem(rp.ctx, rp.key, strconv.Itoa(i)+"_"+delService.Name).Err()
						if err != nil {
							return err
						}
					}
				}
			}

			// 找出配置发生变化的服务
			for name, newService := range newServices {
				if oldService, exists := oldServices[name]; exists {
					if !reflect.DeepEqual(oldService, newService) {
						// fmt.Printf("Service %s config changed from %+v to %+v\n", name, oldService, newService)
						// changed = append(changed, newService)
						if newService.Cap > oldService.Cap {
							// 增加服务
							for i := oldService.Cap + 1; i <= newService.Cap; i++ {
								member := strconv.Itoa(i) + "_" + newService.Name
								err := tx.ZAdd(rp.ctx, rp.key, redis.Z{
									Score:  0, // 默认分数
									Member: member,
								}).Err()
								if err != nil {
									return err
								}
							}
						} else {
							// 减少服务
							for i := newService.Cap + 1; i <= oldService.Cap; i++ {
								err := tx.ZRem(rp.ctx, rp.key, strconv.Itoa(i)+"_"+newService.Name).Err()
								if err != nil {
									return err
								}
							}
						}
					}
				}
			}
			return nil
		}, rp.key)
		if err == redis.TxFailedErr {
			// 有人修改数据重试
			continue
		}
		if err != nil {
			return err
		}
		// 更换成功，更新配置
		rp.conf = newConf
		return nil
	}
}
