package estool

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/ygpkg/yg-go/config"
)

func TestScrollAll(t *testing.T) {
	type Account struct {
		AccountNumber int64  `json:"account_number"`
		Address       string `json:"address"`
		Age           int64  `json:"age"`
		Balance       int64  `json:"balance"`
		City          string `json:"city"`
		Email         string `json:"email"`
		Employer      string `json:"employer"`
		FirstName     string `json:"firstname"`
		Gender        string `json:"gender"`
		LastName      string `json:"lastname"`
		State         string `json:"state"`
	}

	cfg := config.ESConfig{
		Addresses:     []string{"http://localhost:9200"},
		SlowThreshold: time.Millisecond,
	}
	client, initErr := InitES(cfg)
	assert.Nil(t, initErr)

	// 使用方式
	var accounts []Account
	ctx := context.Background()

	queryDSL := `{
		"size": 20,
		"query": {
			"match_all": {}
		}
	}`
	err := NewScrollSearch(client).ScrollAll(ctx,
		"accounts",
		queryDSL,
		&accounts, // 传入指向切片的指针
		WithSize(5),
		WithScrollTime(1*time.Minute),
	)
	assert.Nil(t, err)
	var accountNumberList []int64
	for _, account := range accounts {
		accountNumberList = append(accountNumberList, account.AccountNumber)
	}
	t.Log(accountNumberList)
}
