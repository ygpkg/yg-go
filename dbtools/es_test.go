package dbtools

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitES(t *testing.T) {
	cfg := ESConfig{
		Addr: "http://localhost:9200",
	}
	client, initErr := InitES(cfg)
	assert.Nil(t, initErr)
	ctx := context.Background()
	ctx = context.WithValue(ctx, "requestId", "12312312312312")
	res, searchErr := client.Search(
		client.Search.WithContext(ctx),
		client.Search.WithIndex("accounts"),
		client.Search.WithBody(strings.NewReader(`{"query":{"match_all":{}}}`)),
	)
	assert.Nil(t, searchErr)
	t.Log(res)
}
