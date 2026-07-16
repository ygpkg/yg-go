package tencos

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/tencentyun/cos-go-sdk-v5"
	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/logs"

	storagev2 "github.com/ygpkg/yg-go/storage/v2"
)

func init() {
	storagev2.Register("tencos", func(cfg config.StorageConfig) (storagev2.Storager, error) {
		if cfg.Tencent == nil {
			return nil, fmt.Errorf("tencent cos config is nil")
		}
		return NewTencentCos(*cfg.Tencent, cfg.StorageOption)
	})
}

var _ storagev2.Storager = (*TencentCos)(nil)

type TencentCos struct {
	opt    config.StorageOption
	cosCfg config.TencentCOSConfig
	client *cos.Client
}

func NewTencentCos(cfg config.TencentCOSConfig, opt config.StorageOption) (*TencentCos, error) {
	ustr := fmt.Sprintf("https://%s.cos.%s.myqcloud.com", cfg.Bucket, cfg.Region)
	u, err := url.Parse(ustr)
	if err != nil {
		logs.Errorf("parse url %s error: %v", ustr, err)
		return nil, err
	}
	b := &cos.BaseURL{BucketURL: u}
	c := cos.NewClient(b, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  cfg.TencentConfig.SecretID,
			SecretKey: cfg.TencentConfig.SecretKey,
		},
	})

	return &TencentCos{
		cosCfg: cfg,
		opt:    opt,
		client: c,
	}, nil
}

func (tc *TencentCos) Save(ctx context.Context, fi *storagev2.FileInfo, r io.Reader) error {
	if fi.StoragePath == "" {
		return fmt.Errorf("storage path is empty")
	}
	if r == nil {
		return fmt.Errorf("reader is empty")
	}
	resp, err := tc.client.Object.Put(ctx, fi.StoragePath, r, &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
			ContentType: mime.TypeByExtension(fi.FileExt),
		},
	})
	if err != nil {
		logs.Errorf("tencent cos put object error: %v", err)
		return err
	}
	defer resp.Body.Close()

	if md5str := resp.Header.Get("ETag"); md5str != "" {
		fi.Hash = "md5:" + strings.Trim(md5str, "\"")
	}
	fi.PublicURL = tc.GetPublicURL(fi.StoragePath, false)
	return nil
}

func (tc *TencentCos) GetPublicURL(storagePath string, temp bool) string {
	host := fmt.Sprintf("https://%s.cos.%s.myqcloud.com/", tc.cosCfg.Bucket, tc.cosCfg.Region)
	baseURI := host + storagePath
	if !temp {
		return baseURI
	}
	u, _ := url.Parse(host)
	b := &cos.BaseURL{BucketURL: u}
	client := cos.NewClient(b, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  tc.cosCfg.TencentConfig.SecretID,
			SecretKey: tc.cosCfg.TencentConfig.SecretKey,
			Expire:    tc.opt.PresignedTimeout,
		},
	})
	ctx := context.Background()
	presignedURL, err := client.Object.GetPresignedURL(ctx, http.MethodGet, storagePath,
		tc.cosCfg.TencentConfig.SecretID, tc.cosCfg.TencentConfig.SecretKey, tc.opt.PresignedTimeout, nil)
	if err != nil {
		logs.Errorf("tencent cos get presigned url error: %v", err)
		return ""
	}
	return presignedURL.String()
}

func (tc *TencentCos) GetPresignedURL(method, storagePath string) (string, error) {
	host := fmt.Sprintf("https://%s.cos.%s.myqcloud.com/", tc.cosCfg.Bucket, tc.cosCfg.Region)
	u, _ := url.Parse(host)
	b := &cos.BaseURL{BucketURL: u}
	client := cos.NewClient(b, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  tc.cosCfg.TencentConfig.SecretID,
			SecretKey: tc.cosCfg.TencentConfig.SecretKey,
			Expire:    tc.opt.PresignedTimeout,
		},
	})
	ctx := context.Background()
	presignedURL, err := client.Object.GetPresignedURL(ctx, method, storagePath,
		tc.cosCfg.TencentConfig.SecretID, tc.cosCfg.TencentConfig.SecretKey, tc.opt.PresignedTimeout, nil)
	if err != nil {
		logs.Errorf("tencent cos get presigned url error: %v", err)
		return "", err
	}
	return presignedURL.String(), nil
}

func (tc *TencentCos) ReadFile(storagePath string) (io.ReadCloser, error) {
	if storagePath == "" {
		return nil, fmt.Errorf("storage path is empty")
	}
	resp, err := tc.client.Object.Get(context.Background(), storagePath, nil)
	if err != nil {
		logs.Errorf("tencent cos get object error: %v", err)
		return nil, err
	}
	return resp.Body, nil
}

func (tc *TencentCos) DeleteFile(storagePath string) error {
	if storagePath == "" {
		return fmt.Errorf("storage path is empty")
	}
	resp, err := tc.client.Object.Delete(context.Background(), storagePath)
	if err != nil {
		logs.Errorf("tencent cos delete object error: %v", err)
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (tc *TencentCos) CopyDir(storagePath, dest string) error {
	if storagePath == "" {
		return fmt.Errorf("source storage path is empty")
	}
	if dest == "" {
		return fmt.Errorf("destination storage path is empty")
	}
	isDir, err := tc.isDirectory(storagePath)
	if err != nil {
		logs.Errorf("tencent cos check source path error: %v", err)
		return err
	}
	if isDir {
		return tc.copyDirectory(storagePath, dest)
	}
	return tc.copyObject(storagePath, dest)
}

func (tc *TencentCos) UploadDirectory(localDirPath, destDir string) ([]string, error) {
	return nil, fmt.Errorf("UploadDirectory not implemented for TencentCos")
}

func (tc *TencentCos) isDirectory(storagePath string) (bool, error) {
	_, err := tc.client.Object.Head(context.Background(), storagePath, nil)
	if err != nil {
		if cos.IsNotFoundError(err) {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

func (tc *TencentCos) copyObject(storagePath, dest string) error {
	srcurl := strings.TrimPrefix(tc.GetPublicURL(storagePath, false), "https://")
	_, _, err := tc.client.Object.Copy(context.Background(), dest, srcurl, nil)
	if err != nil {
		logs.Errorf("tencent cos copy object error: %v", err)
		return err
	}
	return nil
}

func (tc *TencentCos) copyDirectory(storagePath, dest string) error {
	opt := &cos.BucketGetOptions{
		Prefix:    storagePath,
		Delimiter: "/",
	}
	res, _, err := tc.client.Bucket.Get(context.Background(), opt)
	if err != nil {
		logs.Errorf("tencent cos list objects error: %v", err)
		return err
	}
	for _, obj := range res.Contents {
		relativePath := strings.TrimPrefix(obj.Key, storagePath)
		targetPath := path.Join(dest, relativePath)
		if err := tc.copyObject(obj.Key, targetPath); err != nil {
			return err
		}
	}
	for _, prefix := range res.CommonPrefixes {
		subFolderPath := strings.TrimPrefix(prefix, storagePath)
		subFolderTargetPath := path.Join(dest, subFolderPath)
		if err := tc.copyDirectory(prefix, subFolderTargetPath); err != nil {
			return err
		}
	}
	return nil
}

func (tc *TencentCos) CreateMultipartUpload(ctx context.Context, in *storagev2.CreateMultipartUploadInput) (*string, error) {
	if in == nil || in.StoragePath == nil || *in.StoragePath == "" {
		return nil, fmt.Errorf("storage path is empty")
	}
	initRst, _, err := tc.client.Object.InitiateMultipartUpload(ctx, *in.StoragePath, nil)
	if err != nil {
		logs.Errorf("tencent cos initiate multipart upload error: %v", err)
		return nil, err
	}
	return &initRst.UploadID, nil
}

func (tc *TencentCos) GeneratePresignedURL(ctx context.Context, in *storagev2.GeneratePresignedURLInput) (*string, error) {
	if in == nil || in.StoragePath == nil || in.UploadID == nil || in.PartNumber == nil {
		return nil, fmt.Errorf("storagePath, uploadID or partNumber is nil")
	}
	host := fmt.Sprintf("https://%s.cos.%s.myqcloud.com/", tc.cosCfg.Bucket, tc.cosCfg.Region)
	u, _ := url.Parse(host)
	b := &cos.BaseURL{BucketURL: u}
	client := cos.NewClient(b, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  tc.cosCfg.TencentConfig.SecretID,
			SecretKey: tc.cosCfg.TencentConfig.SecretKey,
			Expire:    tc.opt.PresignedTimeout,
		},
	})
	params := &url.Values{}
	params.Set("uploadId", *in.UploadID)
	params.Set("partNumber", fmt.Sprintf("%d", *in.PartNumber))
	presignedURL, err := client.Object.GetPresignedURL(ctx, http.MethodPut, *in.StoragePath,
		tc.cosCfg.TencentConfig.SecretID, tc.cosCfg.TencentConfig.SecretKey, tc.opt.PresignedTimeout, params)
	if err != nil {
		logs.Errorf("tencent cos get presigned part url error: %v", err)
		return nil, err
	}
	urlStr := presignedURL.String()
	return &urlStr, nil
}

func (tc *TencentCos) UploadPart(ctx context.Context, in *storagev2.UploadPartInput) (*string, error) {
	if in == nil || in.StoragePath == nil || in.UploadID == nil || in.PartNumber == nil {
		return nil, fmt.Errorf("storagePath, uploadID or partNumber is nil")
	}
	if *in.StoragePath == "" {
		return nil, fmt.Errorf("storage path is empty")
	}
	if in.Data == nil {
		return nil, fmt.Errorf("reader is empty")
	}
	upOpt := &cos.ObjectUploadPartOptions{}
	resp, err := tc.client.Object.UploadPart(ctx, *in.StoragePath, *in.UploadID, *in.PartNumber, in.Data, upOpt)
	if err != nil {
		logs.Errorf("tencent cos upload part error: %v", err)
		return nil, err
	}
	defer resp.Body.Close()
	etag := strings.Trim(resp.Header.Get("ETag"), "\"")
	return &etag, nil
}

func (tc *TencentCos) CompleteMultipartUpload(ctx context.Context, in *storagev2.CompleteMultipartUploadInput) error {
	if in == nil || in.StoragePath == nil || in.UploadID == nil || in.Parts == nil {
		return fmt.Errorf("storagePath, uploadID or parts is nil")
	}
	objs := make([]cos.Object, 0, len(in.Parts.Parts))
	for _, p := range in.Parts.Parts {
		objs = append(objs, cos.Object{PartNumber: int(aws.ToInt32(p.PartNumber)), ETag: aws.ToString(p.ETag)})
	}
	_, _, err := tc.client.Object.CompleteMultipartUpload(ctx, *in.StoragePath, *in.UploadID, &cos.CompleteMultipartUploadOptions{Parts: objs})
	if err != nil {
		logs.Errorf("tencent cos complete multipart upload error: %v", err)
	}
	return err
}

func (tc *TencentCos) AbortMultipartUpload(ctx context.Context, in *storagev2.AbortMultipartUploadInput) error {
	if in == nil || in.StoragePath == nil || in.UploadID == nil {
		return fmt.Errorf("storagePath or uploadID is nil")
	}
	_, err := tc.client.Object.AbortMultipartUpload(ctx, *in.StoragePath, *in.UploadID, nil)
	if err != nil {
		logs.Errorf("tencent cos abort multipart upload error: %v", err)
	}
	return err
}

func (tc *TencentCos) InitUploadTask(ctx context.Context, tempFile *storagev2.TempFile) error {
	initOpt := &cos.InitiateMultipartUploadOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
			XCosMetaXXX: &http.Header{
				"chunk_hash": []string{tempFile.ChunkHash},
			},
		},
	}
	initRst, _, err := tc.client.Object.InitiateMultipartUpload(ctx, tempFile.StoragePath, initOpt)
	if err != nil {
		logs.Errorf("tencent cos initiate multipart upload error: %v", err)
		return err
	}
	tempFile.ThirdUploadID = initRst.UploadID
	return nil
}

func (tc *TencentCos) ListUploadExistsTrunk(ctx context.Context, tempFile *storagev2.TempFile) ([]int, error) {
	listRst, _, err := tc.client.Object.ListParts(ctx, tempFile.StoragePath, tempFile.ThirdUploadID, nil)
	if err != nil {
		logs.Errorf("tencent cos list parts error: %v", err)
		return nil, err
	}
	allPartNumber := make(map[int]struct{})
	for i := 1; i <= int(tempFile.PartCount); i++ {
		allPartNumber[i] = struct{}{}
	}
	exiPartNumber := make([]int, 0, len(listRst.Parts))
	for _, part := range listRst.Parts {
		thisPartNumber := part.PartNumber - 1
		exiPartNumber = append(exiPartNumber, thisPartNumber)
	}
	sort.Ints(exiPartNumber)
	return exiPartNumber, nil
}

func (tc *TencentCos) UploadTrunk(ctx context.Context, tempFile *storagev2.TempFile, partNumber int, r io.Reader, size int64) error {
	upOpt := &cos.ObjectUploadPartOptions{
		ContentLength: size,
	}
	_, err := tc.client.Object.UploadPart(ctx, tempFile.StoragePath, tempFile.ThirdUploadID, partNumber+1, r, upOpt)
	if err != nil {
		logs.Errorf("tencent cos upload part error: %v", err)
		return err
	}
	return nil
}

func (tc *TencentCos) CompleteUploadTask(ctx context.Context, tempFile *storagev2.TempFile) error {
	filename := tempFile.StoragePath
	uploadid := tempFile.ThirdUploadID
	listRst, _, err := tc.client.Object.ListParts(ctx, filename, uploadid, nil)
	if err != nil {
		logs.Errorf("tencent cos list parts error: %v", err)
		return err
	}
	compOpt := &cos.CompleteMultipartUploadOptions{
		Parts: listRst.Parts,
	}
	_, _, err = tc.client.Object.CompleteMultipartUpload(ctx, filename, uploadid, compOpt)
	if err != nil {
		logs.Errorf("tencent cos complete multipart upload error: %v", err)
		return err
	}
	logs.Infof("tencent cos complete multipart upload result")
	return nil
}
