package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
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

// S3Fs .
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
		Bucket:      aws.String(s3fs.s3fsCfg.Bucket),
		Key:         aws.String(fi.StoragePath),
		Body:        r,
		ContentType: aws.String(mime.TypeByExtension(fi.FileExt)),
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

// UploadDirectory 上传本地目录到远程存储
func (sfs *S3Fs) UploadDirectory(localDirPath, destDir string) ([]string, error) {
	var uploadedPaths []string

	// 参数校验
	if localDirPath == "" {
		return nil, fmt.Errorf("local directory path is empty")
	}
	// if destDir == "" {
	// 	return nil, fmt.Errorf("destination storage path is empty")
	// }

	// 遍历本地目录
	err := filepath.WalkDir(localDirPath, func(filePath string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("error accessing path %s: %w", filePath, err)
		}

		// 跳过目录
		if d.IsDir() {
			return nil
		}

		// 获取相对于源目录的相对路径
		relPath, err := filepath.Rel(localDirPath, filePath)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %s: %w", filePath, err)
		}

		// 构造 S3 存储路径（使用正斜杠作为路径分隔符）
		storagePath := path.Join(destDir, relPath)

		// 获取文件扩展名
		fileExt := path.Ext(filePath)

		// 打开文件
		file, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %w", filePath, err)
		}
		defer file.Close() // 确保在本次迭代中关闭文件

		// 构造 FileInfo 结构体
		fi := &FileInfo{
			StoragePath: storagePath,
			FileExt:     fileExt,
		}

		// 上传文件
		if err := sfs.Save(sfs.ctx, fi, file); err != nil {
			return fmt.Errorf("failed to upload file %s to %s: %w", filePath, storagePath, err)
		}

		// 记录上传成功的路径
		uploadedPaths = append(uploadedPaths, storagePath)
		return nil
	})

	// 检查遍历过程中是否有错误
	if err != nil {
		return nil, err
	}

	return uploadedPaths, nil
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
	if sourceKey == destKey {
		return nil
	}
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

func (s3fs *S3Fs) CreateMultipartUpload(ctx context.Context, in *CreateMultipartUploadInput) (*string, error) {
	if in == nil || in.StoragePath == nil || aws.ToString(in.StoragePath) == "" {
		return nil, fmt.Errorf("storage path is empty")
	}
	out, err := s3fs.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket:      s3fs.getBucketName(in.Bucket),
		Key:         in.StoragePath,
		ContentType: in.ContentType,
	})
	if err != nil {
		return nil, err
	}
	return out.UploadId, nil
}
func (s3fs *S3Fs) GeneratePresignedURL(ctx context.Context, in *GeneratePresignedURLInput) (*string, error) {
	if in == nil || in.StoragePath == nil {
		return nil, fmt.Errorf("storagePath, uploadID or partNumber is nil")
	}
	presigner := s3.NewPresignClient(s3fs.client)
	method := aws.ToString(in.Method)
	switch method {
	case http.MethodGet:
		urlReq, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
			Bucket: s3fs.getBucketName(in.Bucket),
			Key:    in.StoragePath,
		}, func(o *s3.PresignOptions) {
			o.Expires = s3fs.opt.PresignedTimeout
		})
		if err != nil {
			return nil, err
		}
		return aws.String(urlReq.URL), nil
	case http.MethodPut:
		if in.UploadID == nil || *in.UploadID == "" {
			urlReq, err := presigner.PresignPutObject(s3fs.ctx, &s3.PutObjectInput{
				Bucket:      s3fs.getBucketName(in.Bucket),
				Key:         in.StoragePath,
				ContentType: in.ContentType,
			}, func(opts *s3.PresignOptions) {
				opts.Expires = s3fs.opt.PresignedTimeout // 链接有效期，默认15分钟，最大不能超过 7 天
			})
			if err != nil {
				return nil, err
			}
			return aws.String(urlReq.URL), nil
		}

		urlReq, err := presigner.PresignUploadPart(ctx, &s3.UploadPartInput{
			Bucket:        s3fs.getBucketName(in.Bucket),
			Key:           in.StoragePath,
			UploadId:      in.UploadID,
			PartNumber:    aws.Int32(int32(*in.PartNumber)),
			ContentMD5:    in.ContentMD5,
			ContentLength: in.ContentLength,
		}, func(o *s3.PresignOptions) {
			o.Expires = s3fs.opt.PresignedTimeout
		})
		if err != nil {
			return nil, err
		}
		return aws.String(urlReq.URL), nil
	default:
		return nil, fmt.Errorf("only GET and PUT are allowed,now: %s", method)
	}
}
func (s3fs *S3Fs) UploadPart(ctx context.Context, in *UploadPartInput) (*string, error) {
	if in == nil || in.StoragePath == nil || in.UploadID == nil || in.PartNumber == nil {
		return nil, fmt.Errorf("storagePath, uploadID or partNumber is nil")
	}
	if in.Data == nil {
		return nil, fmt.Errorf("reader is empty")
	}
	out, err := s3fs.client.UploadPart(ctx, &s3.UploadPartInput{
		Bucket:     s3fs.getBucketName(in.Bucket),
		Key:        in.StoragePath,
		UploadId:   in.UploadID,
		PartNumber: aws.Int32(int32(*in.PartNumber)),
		Body:       in.Data,
	})
	if err != nil {
		return nil, err
	}
	return out.ETag, nil
}
func (s3fs *S3Fs) CompleteMultipartUpload(ctx context.Context, in *CompleteMultipartUploadInput) error {
	if in == nil || in.StoragePath == nil || in.UploadID == nil || in.Parts == nil {
		return fmt.Errorf("storagePath, uploadID or parts is nil")
	}
	_, err := s3fs.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:          s3fs.getBucketName(in.Bucket),
		Key:             in.StoragePath,
		UploadId:        in.UploadID,
		MultipartUpload: in.Parts,
	})
	return err
}
func (s3fs *S3Fs) AbortMultipartUpload(ctx context.Context, in *AbortMultipartUploadInput) error {
	if in == nil || in.StoragePath == nil || in.UploadID == nil {
		return fmt.Errorf("storagePath or uploadID is nil")
	}
	_, err := s3fs.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   s3fs.getBucketName(in.Bucket),
		Key:      in.StoragePath,
		UploadId: in.UploadID,
	})
	return err
}

func (s3fs *S3Fs) getBucketName(bucket *string) *string {
	if bucket != nil && *bucket != "" {
		return bucket
	}
	defaultBucket := s3fs.s3fsCfg.Bucket
	return &defaultBucket
}
