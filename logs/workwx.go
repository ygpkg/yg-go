package logs

import (
	"bytes"
	"fmt"

	"github.com/xen0n/go-workwx"
	"go.uber.org/zap/zapcore"
)

type workwxWriteWyncer struct {
	wxcli *workwx.WebhookClient

	buf *bytes.Buffer
}

var (
	_ zapcore.WriteSyncer = (*workwxWriteWyncer)(nil)
)

// NewWorkwxSyncer 企业微信日志
func NewWorkwxSyncer(wxKey string) zapcore.WriteSyncer {
	ws := &workwxWriteWyncer{
		wxcli: workwx.NewWebhookClient(wxKey),
		buf:   new(bytes.Buffer),
	}
	return ws
}

// Write .
func (c *workwxWriteWyncer) Write(p []byte) (n int, err error) {
	err = c.wxcli.SendTextMessage(string(p), nil)
	if err != nil {
		fmt.Println("send workwx message failed:", err)
		return 0, err
	}
	return len(p), nil
}

func (c *workwxWriteWyncer) Sync() error {
	return nil
}
