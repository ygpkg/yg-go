package logs

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	cls "github.com/tencentcloud/tencentcloud-cls-sdk-go"
	"github.com/ygpkg/yg-go/config"
	"go.uber.org/zap/zapcore"
)

type tencentClsWriteWyncer struct {
	client     *cls.AsyncProducerClient
	topicID    string
	rejected   atomic.Uint64
	pending    atomic.Int64
	failed     atomic.Uint64
	writeMu    sync.RWMutex
	closeOnce  sync.Once
	closed     bool
	closeErr   error
	dropOnFull bool
}

func validateTencentCLS(cfg config.TencentCLSConfig) error {
	for _, field := range []struct{ name, value string }{
		{"endpoint", cfg.Endpoint}, {"secret_id", cfg.SecretID},
		{"secret_key", cfg.SecretKey}, {"topic_id", cfg.TopicID},
	} {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("missing tencent_cls.%s", field.name)
		}
	}
	endpoint, err := url.Parse("http://" + cfg.Endpoint)
	if err != nil || endpoint.Hostname() == "" || endpoint.Host != cfg.Endpoint {
		return errors.New("invalid tencent_cls.endpoint: expected a host with optional port")
	}
	if cfg.MaxBlockSec != nil && *cfg.MaxBlockSec < 0 {
		return errors.New("tencent_cls.max_block_sec must be nonnegative")
	}
	return nil
}

// NewTencentClsSyncer creates a standalone CLS writer managed by Shutdown.
func NewTencentClsSyncer(cfg config.TencentCLSConfig) (zapcore.WriteSyncer, error) {
	if err := validateTencentCLS(cfg); err != nil {
		return nil, err
	}
	configurationGate <- struct{}{}
	defer func() { <-configurationGate }()
	if closing != nil {
		select {
		case <-closing.done:
		default:
			return nil, errors.New("logs shutdown in progress")
		}
	}
	writer, err := newTencentClsSyncer(cfg)
	if err != nil {
		return nil, err
	}
	standaloneClosers = append(standaloneClosers, writer)
	closing = nil
	return writer, nil
}

func newTencentClsSyncer(cfg config.TencentCLSConfig) (managedSyncer, error) {
	producerConfig := cls.GetDefaultAsyncProducerClientConfig()
	producerConfig.Endpoint = cfg.Endpoint
	producerConfig.AccessKeyID = cfg.SecretID
	producerConfig.AccessKeySecret = cfg.SecretKey
	producerConfig.MaxBlockSec = 5
	if cfg.MaxBlockSec != nil {
		producerConfig.MaxBlockSec = *cfg.MaxBlockSec
	}

	producer, err := cls.NewAsyncProducerClient(producerConfig)
	if err != nil {
		// SDK errors are not part of the public diagnostic contract.
		return nil, errors.New("initialize tencent CLS producer")
	}
	producer.Start()
	return &tencentClsWriteWyncer{client: producer, topicID: cfg.TopicID, dropOnFull: producerConfig.MaxBlockSec == 0}, nil
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

	c.writeMu.RLock()
	defer c.writeMu.RUnlock()
	if c.closed {
		return 0, errors.New("tencent CLS writer closed")
	}
	c.pending.Add(1)
	err = c.client.SendLog(c.topicID, log, c)
	if err != nil {
		c.pending.Add(-1)
		if count := c.rejected.Add(1); count&(count-1) == 0 {
			fmt.Fprintf(os.Stderr, "tencent CLS enqueue rejected (total=%d)\n", count)
		}
		if c.dropOnFull {
			return len(p), nil
		}
		return 0, errors.New("tencent CLS enqueue rejected")
	}

	return len(p), nil
}

func (c *tencentClsWriteWyncer) Success(*cls.Result) {
	c.pending.Add(-1)
}

func (c *tencentClsWriteWyncer) Fail(*cls.Result) {
	count := c.failed.Add(1)
	if count&(count-1) == 0 {
		fmt.Fprintf(os.Stderr, "tencent CLS delivery failed (total=%d)\n", count)
	}
	c.pending.Add(-1)
}

func (c *tencentClsWriteWyncer) Close() error {
	return c.closeWithin(legacyCloseTimeout)
}

func (c *tencentClsWriteWyncer) closeWithin(timeout time.Duration) error {
	c.closeOnce.Do(func() {
		// Stop new writes first. Waiting for the write lock is not charged to the
		// drain budget: an in-flight write can sit in SDK backpressure for up to
		// MaxBlockSec. The caller bound is separate (see Shutdown).
		c.writeMu.Lock()
		c.closed = true
		c.writeMu.Unlock()

		deadline := time.Now().Add(timeout)
		var errs []error
		if err := c.client.Close(max(1, time.Until(deadline).Milliseconds())); err != nil {
			errs = append(errs, fmt.Errorf("close tencent CLS producer: %w", err))
		}

		// Keep confirming terminal callbacks while budget remains, even after the
		// SDK close failed: batches already handed to the worker pool usually
		// still finish. SDK v1.0.11 can also observe an empty queue between a
		// worker dequeuing a batch and registering it as in-flight.
		for c.pending.Load() > 0 {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				errs = append(errs, errors.New("tencent CLS delivery still pending at shutdown"))
				break
			}
			time.Sleep(min(remaining, 10*time.Millisecond))
		}
		if failed := c.failed.Load(); failed > 0 {
			errs = append(errs, fmt.Errorf("tencent CLS delivery failed for %d logs during writer lifetime", failed))
		}
		c.closeErr = errors.Join(errs...)
	})
	return c.closeErr
}
