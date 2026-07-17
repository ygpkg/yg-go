package s3

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

	storage "github.com/ygpkg/yg-go/storage/v2"
)

func init() {
	storage.Register("s3", func(cfg config.StorageConfig) (storage.Storager, error) {
		if cfg.S3 == nil {
			return nil, fmt.Errorf("s3 config is nil")
		}
		return NewS3Fs(*cfg.S3, cfg.StorageOption)
	})
}

var _ storage.Storager = (*S3Fs)(nil)

type S3Fs struct {
	opt     config.StorageOption
	s3fsCfg config.S3StorageConfig
	client  *s3.Client
	ctx     context.Context
}

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
		o.BaseEndpoint = aws.String(cfg.EndPoint)
		o.UsePathStyle = cfg.UsePathStyle
	})
	_, err = s3fs.client.HeadBucket(s3fs.ctx,
		&s3.HeadBucketInput{
			Bucket: aws.String(cfg.Bucket),
		})
	if err != nil {
		return nil, err
	}
	return s3fs, nil
}

func (s3fs *S3Fs) Save(ctx context.Context, fi *storage.FileInfo, r io.Reader) error {
	if fi.StoragePath == "" {
		return fmt.Errorf("storage path is empty")
	}
	if r == nil {
		return fmt.Errorf("reader is empty")
	}
	uploader := manager.NewUploader(s3fs.client)
	_, err := uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s3fs.s3fsCfg.Bucket),
		Key:         aws.String(fi.StoragePath),
		Body:        r,
		ContentType: aws.String(mime.TypeByExtension(fi.FileExt)),
	})
	if err != nil {
		return err
	}
	fi.PublicURL = s3fs.GetPublicURL(fi.StoragePath, false)
	return nil
}

func (s3fs *S3Fs) GetPublicURL(storagePath string, _ bool) string {
	if storagePath == "" {
		return ""
	}
	urlObj, err := url.Parse(s3fs.s3fsCfg.EndPoint)
	if err != nil {
		return storagePath
	}
	if s3fs.s3fsCfg.UsePathStyle {
		publicURL, err := url.JoinPath(urlObj.Scheme+"://"+urlObj.Host, s3fs.s3fsCfg.Bucket, storagePath)
		if err != nil {
			return storagePath
		}
		return publicURL
	}
	publicURL, err := url.JoinPath(urlObj.Scheme+"://"+s3fs.s3fsCfg.Bucket+"."+urlObj.Host, storagePath)
	if err != nil {
		return storagePath
	}
	return publicURL
}

func (s3fs *S3Fs) GetPresignedURL(method, storagePath string) (string, error) {
	presigner := s3.NewPresignClient(s3fs.client)
	var (
		err error
		u   *v4.PresignedHTTPRequest
	)

	switch method {
	case http.MethodGet:
		u, err = presigner.PresignGetObject(s3fs.ctx, &s3.GetObjectInput{
			Bucket: aws.String(s3fs.s3fsCfg.Bucket),
			Key:    aws.String(storagePath),
		}, func(opts *s3.PresignOptions) {
			opts.Expires = s3fs.opt.PresignedTimeout
		})
	case http.MethodPut:
		u, err = presigner.PresignPutObject(s3fs.ctx, &s3.PutObjectInput{
			Bucket: aws.String(s3fs.s3fsCfg.Bucket),
			Key:    aws.String(storagePath),
		}, func(opts *s3.PresignOptions) {
			opts.Expires = s3fs.opt.PresignedTimeout
		})
	default:
		return "", fmt.Errorf("only GET and PUT are allowed, now: %s", method)
	}

	if err != nil {
		return "", err
	}
	return u.URL, nil
}

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

func (s3fs *S3Fs) DeleteFile(storagePath string) error {
	if storagePath == "" {
		return fmt.Errorf("storage path is empty")
	}
	_, err := s3fs.client.DeleteObject(s3fs.ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s3fs.s3fsCfg.Bucket),
		Key:    aws.String(storagePath),
	})
	return err
}

func (sfs *S3Fs) CopyDir(storagePath, dest string) error {
	if storagePath == "" {
		return fmt.Errorf("source storage path is empty")
	}
	if dest == "" {
		return fmt.Errorf("destination storage path is empty")
	}
	isDir, err := sfs.isDirectory(storagePath)
	if err != nil {
		return fmt.Errorf("check source path error: %w", err)
	}
	if isDir {
		return sfs.copyDirectory(storagePath, dest)
	}
	return sfs.copyObject(storagePath, dest)
}

func (sfs *S3Fs) UploadDirectory(localDirPath, destDir string) ([]string, error) {
	var uploadedPaths []string
	if localDirPath == "" {
		return nil, fmt.Errorf("local directory path is empty")
	}
	err := filepath.WalkDir(localDirPath, func(filePath string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("error accessing path %s: %w", filePath, err)
		}
		if d.IsDir() {
			return nil
		}
		relPath, err := filepath.Rel(localDirPath, filePath)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %s: %w", filePath, err)
		}
		storagePath := path.Join(destDir, relPath)
		fileExt := path.Ext(filePath)
		file, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %w", filePath, err)
		}
		defer file.Close()
		fi := &storage.FileInfo{
			StoragePath: storagePath,
			FileExt:     fileExt,
		}
		if err := sfs.Save(sfs.ctx, fi, file); err != nil {
			return fmt.Errorf("failed to upload file %s to %s: %w", filePath, storagePath, err)
		}
		uploadedPaths = append(uploadedPaths, storagePath)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return uploadedPaths, nil
}

func (sfs *S3Fs) isDirectory(storagePath string) (bool, error) {
	_, err := sfs.client.HeadObject(sfs.ctx, &s3.HeadObjectInput{
		Bucket: aws.String(sfs.s3fsCfg.Bucket),
		Key:    aws.String(storagePath),
	})
	if err != nil {
		var notFound *types.NotFound
		if ok := errors.As(err, &notFound); ok {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

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
			if err = sfs.copyObject(aws.ToString(obj.Key), destKey); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s3fs *S3Fs) CreateMultipartUpload(ctx context.Context, in *storage.CreateMultipartUploadInput) (*string, error) {
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

func (s3fs *S3Fs) GeneratePresignedURL(ctx context.Context, in *storage.GeneratePresignedURLInput) (*string, error) {
	if in == nil || in.StoragePath == nil {
		return nil, fmt.Errorf("storagePath is nil")
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
				opts.Expires = s3fs.opt.PresignedTimeout
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
		return nil, fmt.Errorf("only GET and PUT are allowed, now: %s", method)
	}
}

func (s3fs *S3Fs) UploadPart(ctx context.Context, in *storage.UploadPartInput) (*string, error) {
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

func (s3fs *S3Fs) CompleteMultipartUpload(ctx context.Context, in *storage.CompleteMultipartUploadInput) error {
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

func (s3fs *S3Fs) AbortMultipartUpload(ctx context.Context, in *storage.AbortMultipartUploadInput) error {
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
