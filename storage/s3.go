package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	s3config "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/ygpkg/yg-go/config"
)

var _ Storager = (*S3Fs)(nil)

// MinFs .
type S3Fs struct {
	opt     config.StorageOption
	s3fsCfg config.S3StorageConfig
	client  *s3.Client
	ctx     context.Context
}

// NewS3Fs 初始化S3Fs
func NewS3Fs(cfg config.S3StorageConfig, opt config.StorageOption) (*S3Fs, error) {
	if cfg.Bucket == "" {
		return nil, errors.New("configuration bucket error")
	}

	s3fs := &S3Fs{
		opt:     opt,
		s3fsCfg: cfg,
		ctx:     context.Background(),
	}
	s3cfg, err := s3config.LoadDefaultConfig(s3fs.ctx,
		s3config.WithRegion(cfg.Region),
		s3config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")),
	)
	if err != nil {
		return nil, err
	}

	s3fs.client = s3.NewFromConfig(s3cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.EndPoint) // 直接指定 endpoint URL
		o.UsePathStyle = cfg.UsePathStyle         // 使用路径风格的URL
		// 类似es配置可导入日志配置
	})
	// 检查存储桶是否存在
	_, err = s3fs.client.HeadBucket(s3fs.ctx,
		&s3.HeadBucketInput{
			Bucket: aws.String(cfg.Bucket),
		})
	if err != nil {
		return nil, err
	}
	return s3fs, nil
}

// Save 保存文件
func (s3fs *S3Fs) Save(ctx context.Context, fi *FileInfo, r io.Reader) error {
	if fi.StoragePath == "" {
		return fmt.Errorf("storage path is empty")
	}
	if r == nil {
		return fmt.Errorf("reader is empty")
	}
	uploader := manager.NewUploader(s3fs.client)
	_, err := uploader.Upload(s3fs.ctx, &s3.PutObjectInput{
		Bucket: aws.String(s3fs.s3fsCfg.Bucket),
		Key:    aws.String(fi.StoragePath),
		Body:   r,
	})
	if err != nil {
		return err
	}
	return nil
}

// GetPublicURL 获取公共URL
func (s3fs *S3Fs) GetPublicURL(storagePath string, _ bool) string {
	if storagePath == "" {
		return ""
	}
	url_obj, err := url.Parse(s3fs.s3fsCfg.EndPoint)
	if err != nil {
		return storagePath
	}
	if s3fs.s3fsCfg.UsePathStyle {
		// minio
		public_url, err := url.JoinPath(url_obj.Scheme+"://"+url_obj.Host, s3fs.s3fsCfg.Bucket, storagePath)
		if err != nil {
			return storagePath
		}
		return public_url
	}
	// cos
	public_url, err := url.JoinPath(url_obj.Scheme+"://"+s3fs.s3fsCfg.Bucket+"."+url_obj.Host, storagePath)
	if err != nil {
		return storagePath
	}
	// 官方方法
	// aa, _ := s3fs.client.Options().EndpointResolverV2.ResolveEndpoint(s3fs.ctx, s3.EndpointParameters{
	// 	Bucket:         aws.String(s3fs.s3fsCfg.Bucket),
	// 	Region:         aws.String(s3fs.s3fsCfg.Region),
	// 	Endpoint:       aws.String(s3fs.s3fsCfg.EndPoint),
	// 	ForcePathStyle: aws.Bool(s3fs.s3fsCfg.UsePathStyle),
	// })
	return public_url
}

// GetPresignedURL 获取预签名URL
func (s3fs *S3Fs) GetPresignedURL(method, storagePath string) (string, error) {
	presigner := s3.NewPresignClient(s3fs.client)
	var (
		err error
		url *v4.PresignedHTTPRequest
	)

	switch method {
	case http.MethodGet:
		url, err = presigner.PresignGetObject(s3fs.ctx, &s3.GetObjectInput{
			Bucket: aws.String(s3fs.s3fsCfg.Bucket),
			Key:    aws.String(storagePath),
		}, func(opts *s3.PresignOptions) {
			opts.Expires = s3fs.opt.PresignedTimeout // 链接有效期，默认15分钟，最大不能超过 7 天
		})
	case http.MethodPut:
		url, err = presigner.PresignPutObject(s3fs.ctx, &s3.PutObjectInput{
			Bucket: aws.String(s3fs.s3fsCfg.Bucket),
			Key:    aws.String(storagePath),
		}, func(opts *s3.PresignOptions) {
			opts.Expires = s3fs.opt.PresignedTimeout // 链接有效期，默认15分钟，最大不能超过 7 天
		})
	default:
		return "", fmt.Errorf("only GET and PUT are allowed,now: %s", method)
	}

	if err != nil {
		return "", err
	}

	return url.URL, nil
}

// ReadFile 获取文件内容
func (s3fs *S3Fs) ReadFile(storagePath string) (io.ReadCloser, error) {
	if storagePath == "" {
		return nil, fmt.Errorf("storage path is empty")
	}
	obj, err := s3fs.client.GetObject(s3fs.ctx, &s3.GetObjectInput{
		Bucket: aws.String(s3fs.s3fsCfg.Bucket),
		Key:    aws.String(storagePath),
	})
	if err != nil {
		return nil, err
	}
	return obj.Body, nil
}

// DeleteFile 删除文件
func (s3fs *S3Fs) DeleteFile(storagePath string) error {
	if storagePath == "" {
		return fmt.Errorf("storage path is empty")
	}
	_, err := s3fs.client.DeleteObject(s3fs.ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s3fs.s3fsCfg.Bucket),
		Key:    aws.String(storagePath),
	})
	if err != nil {
		return err
	}
	return nil
}

// CopyDir 复制文件或文件夹
func (sfs *S3Fs) CopyDir(storagePath, dest string) error {
	if storagePath == "" {
		return fmt.Errorf("source storage path is empty")
	}
	if dest == "" {
		return fmt.Errorf("destination storage path is empty")
	}

	// 检查源路径是文件还是文件夹
	isDir, err := sfs.isDirectory(storagePath)
	if err != nil {
		return fmt.Errorf("check source path error: %w", err)
	}

	if isDir {
		// 复制整个目录
		return sfs.copyDirectory(storagePath, dest)
	} else {
		// 复制单个文件
		return sfs.copyObject(storagePath, dest)
	}
}

// isDirectory 判断路径是否为目录（如果 HeadObject 失败且不存在，则认为是目录）
func (sfs *S3Fs) isDirectory(storagePath string) (bool, error) {
	_, err := sfs.client.HeadObject(sfs.ctx, &s3.HeadObjectInput{
		Bucket: aws.String(sfs.s3fsCfg.Bucket),
		Key:    aws.String(storagePath),
	})

	if err != nil {
		var notFound *types.NotFound
		if ok := errors.As(err, &notFound); ok {
			// 对象不存在，假设是目录
			return true, nil
		}
		return false, err
	}

	// 文件存在，不是目录
	return false, nil
}

// copyObject 单个文件复制
func (sfs *S3Fs) copyObject(sourceKey, destKey string) error {
	src := fmt.Sprintf("%s/%s", sfs.s3fsCfg.Bucket, sourceKey)

	_, err := sfs.client.CopyObject(sfs.ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(sfs.s3fsCfg.Bucket),
		Key:        aws.String(destKey),
		CopySource: aws.String(src),
	})

	if err != nil {
		return fmt.Errorf("copy object %s to %s error: %w", sourceKey, destKey, err)
	}

	return nil
}

// copyDirectory 递归复制整个目录下的所有对象
func (sfs *S3Fs) copyDirectory(sourcePrefix, destPrefix string) error {
	paginator := s3.NewListObjectsV2Paginator(sfs.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(sfs.s3fsCfg.Bucket),
		Prefix: aws.String(sourcePrefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(sfs.ctx)
		if err != nil {
			return fmt.Errorf("list objects error: %w", err)
		}

		for _, obj := range page.Contents {
			relativePath := strings.TrimPrefix(aws.ToString(obj.Key), sourcePrefix)
			destKey := path.Join(destPrefix, relativePath)
			if destKey == destPrefix {
				continue
			}
			err = sfs.copyObject(aws.ToString(obj.Key), destKey)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
