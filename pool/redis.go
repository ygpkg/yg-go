package pool

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/dbtools/redispool"
	"github.com/ygpkg/yg-go/logs"
)

var _ Pool = (*RedisPool)(nil)

// RedisPool 实现 Pool 接口
type RedisPool struct {
	cli    *redis.Client
	ctx    context.Context
	rdsKey string                   // 服务名
	conf   config.ServicePoolConfig //服务配置
}

// NewRedisPool 创建一个新的 服务池 每次新建一个服务池时，会清空原有服务池
func NewRedisPool(ctx context.Context, cli *redis.Client, rdsKey string, conf config.ServicePoolConfig) *RedisPool {
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
	err := cli.ZAdd(ctx, rdsKey, zsetMembers...).Err()
	if err != nil {
		logs.Errorf("zadd error", err)
		panic(err)
	}
	logs.Infof("new redis pool success")

	return &RedisPool{
		ctx:    ctx,
		cli:    cli,
		rdsKey: rdsKey,
		conf:   conf,
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
	members, err := rp.cli.ZPopMin(rp.ctx, rp.rdsKey, 1).Result()
	if err != nil {
		logs.Errorf("found service member failed, %s", err.Error())
		return nil, err
	}

	if len(members) == 0 {
		logs.Errorf("no service member %s", rp.rdsKey)
		return nil, fmt.Errorf("no service %s", rp.rdsKey)
	}

	minScore := members[0].Score
	if minScore > nowNano {
		// 没有空闲的放回去
		err = rp.cli.ZAdd(rp.ctx, rp.rdsKey, members[0]).Err()
		if err != nil {
			return nil, err
		}
		logs.Errorf("acquire member %s failed, error %s", members[0], err)
		return "", fmt.Errorf("no service %s", rp.rdsKey)
	}
	members[0].Score = futureNano
	// 后延时间
	err = rp.cli.ZAdd(rp.ctx, rp.rdsKey, members[0]).Err()
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
			score, err := tx.ZScore(rp.ctx, rp.rdsKey, rz.Member.(string)).Result()
			if err == redis.Nil {
				logs.Errorf("member does not exist.%s", err.Error())
				return fmt.Errorf("member does not exist.%s", err.Error())
			} else if err != nil {
				logs.Errorf("ZScore error.%s", err.Error())
				return err
			}

			// 检查分数是否匹配
			if score == rz.Score {
				// 相等
				// 开启事务
				_, err := tx.Pipelined(rp.ctx, func(pipe redis.Pipeliner) error {
					// 归还，设置默认值
					rz.Score = 0
					return pipe.ZAdd(rp.ctx, rp.rdsKey, rz).Err()
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

		}, rp.rdsKey)
		if err == redis.TxFailedErr {
			logs.Errorf("Someone modified the data and tried again: %s", err.Error())
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
	rp.cli.ZRemRangeByRank(rp.ctx, rp.rdsKey, 0, -1)
}

// RefreshConfig 刷新配置
func (rp *RedisPool) RefreshConfig(newConf config.ServicePoolConfig) error {
	// 获取旧配置和新配置的所有成员
	oldMembers := generateMembers(rp.conf.Services)
	newMembers := generateMembers(newConf.Services)

	// 找出新增的和被删除的成员
	membersToAdd := diff(newMembers, oldMembers)
	membersToRemove := diff(oldMembers, newMembers)

	// 锁
	err := redispool.Lock(rp.key, 5*time.Second)
	if err != nil {
		logs.Errorf("refreshConfig failed to lock")
		return err
	}
	defer redispool.UnLock(rp.key)

	// 批量添加和删除
	if len(membersToAdd) > 0 {
		zAddOps := make([]redis.Z, 0, len(membersToAdd))
		for _, member := range membersToAdd {
			zAddOps = append(zAddOps, redis.Z{Score: 0, Member: member})
		}
		if err := rp.cli.ZAdd(rp.ctx, rp.key, zAddOps...).Err(); err != nil {
			logs.Errorf("failed to add members: %w", err)
			return fmt.Errorf("failed to add members: %w", err)
		}
	}
	if len(membersToRemove) > 0 {
		// 将 []string 转为 []interface{}
		membersToRemoveInterface := make([]interface{}, len(membersToRemove))
		for i, member := range membersToRemove {
			membersToRemoveInterface[i] = member
		}
		if err := rp.cli.ZRem(rp.ctx, rp.key, membersToRemoveInterface...).Err(); err != nil {
			logs.Errorf("failed to remove members: %w", err)
			return fmt.Errorf("failed to remove members: %w", err)
		}
	}
	redispool.UnLock(rp.key)
	return nil
}

// generateMembers 生成服务对应的所有成员
func generateMembers(services []config.ServiceInfo) map[string]struct{} {
	members := make(map[string]struct{})
	for _, service := range services {
		for i := 1; i <= service.Cap; i++ {
			member := strconv.Itoa(i) + "_" + service.Name
			members[member] = struct{}{}
		}
	}
	return members
}

// diff 返回在 source 中存在，但在 target 中不存在的元素
func diff(source, target map[string]struct{}) []string {
	result := []string{}
	for member := range source {
		if _, exists := target[member]; !exists {
			result = append(result, member)
		}
	}
	return result
}
