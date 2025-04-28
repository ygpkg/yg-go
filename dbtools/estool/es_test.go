package estool

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/ygpkg/yg-go/config"
)

func TestInitES(t *testing.T) {
	cfg := config.ESConfig{
		Addresses:     []string{"http://localhost:9200"},
		SlowThreshold: time.Millisecond,
	}
	client, initErr := InitES(cfg)
	assert.Nil(t, initErr)
	ctx := context.Background()
	ctx = context.WithValue(ctx, "reqid", "12312312312312")
	res, searchErr := client.Search(
		client.Search.WithContext(ctx),
		client.Search.WithIndex("accounts"),
		client.Search.WithBody(strings.NewReader(`{"query":{"match_all":{}}}`)),
	)
	assert.Nil(t, searchErr)
	t.Log(res)
}
