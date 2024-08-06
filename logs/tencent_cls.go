package logs

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	cls "github.com/tencentcloud/tencentcloud-cls-sdk-go"
	"github.com/ygpkg/yg-go/config"
	"go.uber.org/zap/zapcore"
)

const (
	tencentClsMaxBatchSize = 20
)

type tencentClsWriteWyncer struct {
	client *cls.AsyncProducerClient
	cfg    config.TencentCLSConfig
	sync.Mutex
}

var (
	_ zapcore.WriteSyncer = (*tencentClsWriteWyncer)(nil)
)

// NewTencentClsSyncer 阿里云日志服务
func NewTencentClsSyncer(cfg config.TencentCLSConfig) (zapcore.WriteSyncer, error) {
	producerConfig := cls.GetDefaultAsyncProducerClientConfig()
	producerConfig.Endpoint = cfg.Endpoint
	producerConfig.AccessKeyID = cfg.SecretID
	producerConfig.AccessKeySecret = cfg.SecretKey
	producerConfig.MaxBlockSec = 5

	producerInstance, err := cls.NewAsyncProducerClient(producerConfig)
	if err != nil {
		fmt.Println("create tencent cls producer failed:", err)
		return nil, err
	}
	producerInstance.Start()

	log := &tencentClsWriteWyncer{
		client: producerInstance,
		cfg:    cfg,
	}
	closerList = append(closerList, log)
	return log, nil
}

func (c *tencentClsWriteWyncer) Sync() error {
	return nil
}

// Write .
func (c *tencentClsWriteWyncer) Write(p []byte) (n int, err error) {
	data := map[string]interface{}{}
	err = json.Unmarshal(p, &data)
	if err != nil {
		fmt.Println("unmarshal log failed:", err)
		return 0, err
	}

	addLogMap := map[string]string{}
	for k, v := range data {
		addLogMap[k] = fmt.Sprint(v)
	}
	log := cls.NewCLSLog(time.Now().Unix(), addLogMap)

	err = c.client.SendLog(c.cfg.TopicID, log, c)
	if err != nil {
		fmt.Println("send tencent cls message failed:", err)
		return 0, err
	}

	return len(p), nil
}

func (c *tencentClsWriteWyncer) Success(result *cls.Result) {
	fmt.Println("send tencent cls message success:", result)
}
func (c *tencentClsWriteWyncer) Fail(result *cls.Result) {
	fmt.Println("send tencent cls message failed:", result)
}

func (c *tencentClsWriteWyncer) Close() error {
	c.client.Close(500)
	return nil
}
