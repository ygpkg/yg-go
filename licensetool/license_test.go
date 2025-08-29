package licensetool

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"
)

// TestHashConsistency 验证哈希计算的一致性
func TestHashConsistency(t *testing.T) {
	// 1. 准备数据和密钥
	// 密钥可以是任何 byte 切片，这里我们用一个简单的哈希作为密钥
	secretData := "license_seed_12345"
	key := sha256.Sum256([]byte(secretData))

	now := time.Now().String()

	// 2. 第一次哈希计算
	hm1 := hmac.New(sha256.New, key[:])
	hm1.Write([]byte(now))
	hash1 := hex.EncodeToString(hm1.Sum(nil))

	// 3. 第二次哈希计算
	// 使用同样的数据和密钥
	hm2 := hmac.New(sha256.New, key[:])
	hm2.Write([]byte(now))
	hash2 := hex.EncodeToString(hm2.Sum(nil))

	// 4. 验证结果是否相同
	if hash1 != hash2 {
		t.Errorf("哈希计算不一致！\n期望哈希: %s\n实际哈希: %s", hash1, hash2)
	}

	fmt.Println("哈希计算一致性测试通过。")
	fmt.Printf("哈希结果1: %s\n", hash1)
	fmt.Printf("哈希结果2: %s\n", hash2)
}

func Hash(key [32]byte, data []byte) string {
	hm2 := hmac.New(sha256.New, key[:])
	hm2.Write(data)
	return hex.EncodeToString(hm2.Sum(nil))
}

func TestGenHash(t *testing.T) {
	hash := Hash(sha256.Sum256([]byte("3e3e6f57-2150-4d00-9f8b-7dd1c1262668"+"eb0ba985-b57f-4fe1-8081-10f3950edc02")),
		[]byte("2025-08-29 11:41:05"+"ffd19930daac67d4b3574fad63b8e0d8ceea5a68283dee28cc0e6e9b01c3a730"))
	fmt.Println(hash)
	oriHash := "cd051a85e83b93b535ddffd577e11b34ea901f8d79b4be7ba5c67b42659bac96"
	fmt.Println(oriHash)
	fmt.Println(hash == oriHash)
}
