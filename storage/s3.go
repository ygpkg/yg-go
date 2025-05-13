package storage

import (
	"context"
	"log"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

var (
	bucket          = "yourbucketname"
	region          = "us-east-1"
	accessKeyID     = "youraccesskeyid"
	secretAccessKey = "yoursecretaccesskey"
)

func InitS3Client() *s3.Client {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, "")),
	)
	if err != nil {
		log.Fatalf("unable to load SDK config, %v", err)
	}
	return s3.NewFromConfig(cfg)
}
