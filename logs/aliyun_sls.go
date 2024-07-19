package logs

// import (
// 	"encoding/json"
// 	"fmt"
// 	"time"

// 	"sync"

// 	sls "github.com/aliyun/aliyun-log-go-sdk"
// 	"github.com/ygpkg/yg-go/config"
// 	"go.uber.org/zap/zapcore"
// 	"google.golang.org/protobuf/proto"
// )

// const (
// 	aliyunSlsMaxBatchSize = 20
// )

// type aliyunSlsWriteWyncer struct {
// 	client sls.ClientInterface
// 	cfg    config.AliyunSLSConfig
// 	buf    []*sls.Log
// 	sync.Mutex
// }

// var (
// 	_ zapcore.WriteSyncer = (*aliyunSlsWriteWyncer)(nil)
// )

// // NewAliyunSlsSyncer 阿里云日志服务
// func NewAliyunSlsSyncer(cfg config.AliyunSLSConfig) zapcore.WriteSyncer {
// 	client := sls.CreateNormalInterfaceV2(
// 		cfg.Endpoint,
// 		sls.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.AccessKeySecret, ""),
// 	)

// 	alilog := &aliyunSlsWriteWyncer{
// 		client: client,
// 		cfg:    cfg,
// 		buf:    make([]*sls.Log, 0, aliyunSlsMaxBatchSize),
// 	}
// 	return alilog
// }

// // Write .
// func (c *aliyunSlsWriteWyncer) Write(p []byte) (n int, err error) {
// 	data := map[string]interface{}{}
// 	err = json.Unmarshal(p, &data)
// 	if err != nil {
// 		fmt.Println("unmarshal log failed:", err)
// 		return 0, err
// 	}

// 	log := &sls.Log{
// 		Time:     proto.Uint32(uint32(time.Now().Unix())),
// 		Contents: make([]*sls.LogContent, 0, len(data)),
// 	}

// 	for k, v := range data {
// 		log.Contents = append(log.Contents, &sls.LogContent{
// 			Key:   String(k),
// 			Value: String(fmt.Sprintf("%v", v)),
// 		})
// 	}
// 	c.Lock()
// 	c.buf = append(c.buf, log)
// 	c.Unlock()

// 	if len(c.buf) >= aliyunSlsMaxBatchSize {
// 		c.Sync()
// 	}

// 	return len(p), nil
// }

// func (c *aliyunSlsWriteWyncer) Sync() error {
// 	c.Lock()
// 	defer c.Unlock()
// 	lg := &sls.LogGroup{
// 		Topic:    proto.String("tp1"),
// 		Source:   proto.String("10.230.201.117"),
// 		Category: proto.String("test"),
// 		Logs:     c.buf,
// 	}
// 	err := c.client.PutLogs(c.cfg.Project, c.cfg.Logstore, lg)
// 	if err != nil {
// 		fmt.Println("send aliyun sls message failed:", err)
// 		return err
// 	}
// 	c.buf = c.buf[:0]
// 	return nil
// }

// // String is a helper routine that allocates a new string value
// // to store v and returns a pointer to it.
// func String(v string) *string {
// 	p := new(string)
// 	*p = v
// 	return p
// }

// // StringVal is a helper routine that allocates a new string value
// func StringVal(v interface{}) *string {
// 	p := new(string)
// 	*p = fmt.Sprintf("%v", v)
// 	return p
// }
