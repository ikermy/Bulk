package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"time"

	"github.com/ikermy/Bulk/internal/metrics"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// FileClient is a simple S3/MinIO-backed storage client
type FileClient struct {
	client    *minio.Client
	// testClient is an optional test-only client that can be used in unit tests
	// to avoid depending on a real minio.Client. If non-nil, FileClient methods
	// will use it instead of the real client.
	testClient minioClientIface
	bucket    string
	baseURL   string // optional public base URL to construct file URLs
	useObject bool
}

// Client is a minimal interface used by other packages to interact with file storage
type Client interface {
	Save(name string, r io.Reader) (string, error)
	Presign(name string, expiry time.Duration) (string, error)
	Get(name string) (io.ReadCloser, error)
	PublicURL(name string) (string, error)
}

// NewFileClientFromEnv constructs FileClient using environment variables:
//
//	STORAGE_ENDPOINT, STORAGE_ACCESS_KEY, STORAGE_SECRET_KEY, STORAGE_BUCKET, STORAGE_USE_SSL (bool), STORAGE_BASE_URL (optional)
func NewFileClientFromEnv() (*FileClient, error) {
	endpoint := os.Getenv("STORAGE_ENDPOINT")
	access := os.Getenv("STORAGE_ACCESS_KEY")
	secret := os.Getenv("STORAGE_SECRET_KEY")
	bucket := os.Getenv("STORAGE_BUCKET")
	useSSL := false
	if os.Getenv("STORAGE_USE_SSL") == "true" {
		useSSL = true
	}
	baseURL := os.Getenv("STORAGE_BASE_URL")
	if endpoint == "" || access == "" || secret == "" || bucket == "" {
		return nil, fmt.Errorf("storage configuration missing (STORAGE_ENDPOINT/ACCESS_KEY/SECRET_KEY/BUCKET)")
	}

	// allow tests to override minio.New by replacing minioNew variable
	minioClient, err := minioNew(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(access, secret, ""),
		Secure: useSSL,
	})
	if err != nil {
		metrics.StorageErrorsTotal.WithLabelValues("init", "error").Inc()
		return nil, err
	}

	// ensure bucket exists
	ctx := context.Background()
	exists, err := minioClient.BucketExists(ctx, bucket)
	if err != nil {
		return nil, err
	}
	if !exists {
		if err := minioClient.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, err
		}
	}

	return &FileClient{client: minioClient, bucket: bucket, baseURL: baseURL, useObject: true}, nil
}

// minioNew is a variable wrapper around minio.New. Tests can replace this
// variable to simulate minio.New failures without changing production code.
var minioNew = minio.New

// minioClientIface is a lightweight subset of methods we use on the minio
// client for unit testing. It is intentionally slightly different from the
// concrete minio.Client signatures for GetObject to allow returning an
// io.ReadCloser in tests.
type minioClientIface interface {
	PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, opts minio.PutObjectOptions) (minio.UploadInfo, error)
	PresignedGetObject(ctx context.Context, bucketName, objectName string, expires time.Duration, reqParams url.Values) (*url.URL, error)
	GetObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (io.ReadCloser, error)
	BucketExists(ctx context.Context, bucketName string) (bool, error)
	MakeBucket(ctx context.Context, bucketName string, opts minio.MakeBucketOptions) error
}

// Save uploads the content from reader to storage under given name and returns an object URL (or object name as ID)
func (f *FileClient) Save(name string, r io.Reader) (string, error) {
	if f == nil || !f.useObject {
		return "", fmt.Errorf("storage client not configured")
	}
	ctx := context.Background()
	// ensure directory-like path is sanitized
	key := path.Clean(name)
	// choose content type auto-detect omitted; use application/octet-stream
	metrics.StorageOperationsTotal.WithLabelValues("putobject", "attempt").Inc()
	var info minio.UploadInfo
	var err error
	if f.testClient != nil {
		info, err = f.testClient.PutObject(ctx, f.bucket, key, r, -1, minio.PutObjectOptions{ContentType: "application/octet-stream"})
	} else {
		if f.client == nil {
			return "", fmt.Errorf("storage client not configured")
		}
		info, err = f.client.PutObject(ctx, f.bucket, key, r, -1, minio.PutObjectOptions{ContentType: "application/octet-stream"})
	}
	if err != nil {
		metrics.StorageErrorsTotal.WithLabelValues("putobject", "error").Inc()
		metrics.StorageOperationsTotal.WithLabelValues("putobject", "error").Inc()
		return "", err
	}
	metrics.StorageOperationsTotal.WithLabelValues("putobject", "success").Inc()
	// return object key as ID (callers can construct public URL or presign)
	return info.Key, nil
}

// Presign returns a presigned GET URL for object name with given expiry
func (f *FileClient) Presign(name string, expiry time.Duration) (string, error) {
	if f == nil || !f.useObject {
		return "", fmt.Errorf("storage client not configured")
	}
	ctx := context.Background()
	key := path.Clean(name)
	// empty params
	params := url.Values{}
	metrics.StorageOperationsTotal.WithLabelValues("presign", "attempt").Inc()
	var u *url.URL
	var err error
	if f.testClient != nil {
		u, err = f.testClient.PresignedGetObject(ctx, f.bucket, key, expiry, params)
	} else {
		if f.client == nil {
			return "", fmt.Errorf("storage client not configured")
		}
		u, err = f.client.PresignedGetObject(ctx, f.bucket, key, expiry, params)
	}
	if err != nil {
		metrics.StorageErrorsTotal.WithLabelValues("presign", "error").Inc()
		metrics.StorageOperationsTotal.WithLabelValues("presign", "error").Inc()
		return "", err
	}
	metrics.StorageOperationsTotal.WithLabelValues("presign", "success").Inc()
	return u.String(), nil
}

// Get returns a reader for the stored object
func (f *FileClient) Get(name string) (io.ReadCloser, error) {
	if f == nil || !f.useObject {
		return nil, fmt.Errorf("storage client not configured")
	}
	ctx := context.Background()
	key := path.Clean(name)
	metrics.StorageOperationsTotal.WithLabelValues("getobject", "attempt").Inc()
	var obj io.ReadCloser
	var err error
	if f.testClient != nil {
		obj, err = f.testClient.GetObject(ctx, f.bucket, key, minio.GetObjectOptions{})
	} else {
		if f.client == nil {
			return nil, fmt.Errorf("storage client not configured")
		}
		obj, err = f.client.GetObject(ctx, f.bucket, key, minio.GetObjectOptions{})
	}
	if err != nil {
		metrics.StorageErrorsTotal.WithLabelValues("getobject", "error").Inc()
		metrics.StorageOperationsTotal.WithLabelValues("getobject", "error").Inc()
		return nil, err
	}
	metrics.StorageOperationsTotal.WithLabelValues("getobject", "success").Inc()
	return obj, nil
}

// PublicURL constructs a public URL using baseURL if configured
func (f *FileClient) PublicURL(name string) (string, error) {
	if f == nil {
		return "", fmt.Errorf("storage client not configured")
	}
	if f.baseURL == "" {
		return "", fmt.Errorf("baseURL not configured")
	}
	u, err := url.Parse(f.baseURL)
	if err != nil {
		return "", err
	}
	u.Path = path.Join(u.Path, f.bucket, path.Clean(name))
	return u.String(), nil
}
