package encryptor

import uuid "github.com/satori/go.uuid"

// UUID generates a random UUID according to RFC 4122
func UUID() string {
	return GenerateUUID()
}

// GenerateUUID 生成UUID
func GenerateUUID() string {
	return uuid.Must(uuid.NewV4(), nil).String()
}
