package storage

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/require"
)

// fakeMinio implements the minioClientIface used by FileClient tests.
type fakeMinio struct{}

func (f *fakeMinio) PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	// read input to simulate upload
	_, _ = ioutil.ReadAll(reader)
	return minio.UploadInfo{Key: objectName}, nil
}
func (f *fakeMinio) PresignedGetObject(ctx context.Context, bucketName, objectName string, expires time.Duration, reqParams url.Values) (*url.URL, error) {
	u, _ := url.Parse("https://example.com/" + bucketName + "/" + objectName)
	return u, nil
}
func (f *fakeMinio) GetObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (io.ReadCloser, error) {
	return ioutil.NopCloser(strings.NewReader("data")), nil
}
func (f *fakeMinio) BucketExists(ctx context.Context, bucketName string) (bool, error) {
	return true, nil
}
func (f *fakeMinio) MakeBucket(ctx context.Context, bucketName string, opts minio.MakeBucketOptions) error {
	return nil
}

func TestPublicURL_NoBaseURL(t *testing.T) {
	f := &FileClient{bucket: "b"}
	_, err := f.PublicURL("x")
	require.Error(t, err)
}

func TestPublicURL_Success(t *testing.T) {
	f := &FileClient{bucket: "b", baseURL: "https://cdn.example.com/base"}
	u, err := f.PublicURL("/path/to/file.txt")
	require.NoError(t, err)
	require.Contains(t, u, "https://cdn.example.com/base/b/path/to/file.txt")
}

func TestSave_ClientNotConfigured(t *testing.T) {
	f := &FileClient{useObject: false}
	_, err := f.Save("k", strings.NewReader("x"))
	require.Error(t, err)
}

func TestSave_Success_WithFakeMinio(t *testing.T) {
	f := &FileClient{bucket: "b", useObject: true, testClient: &fakeMinio{}}
	key, err := f.Save("my/object.txt", strings.NewReader("hello"))
	require.NoError(t, err)
	require.Equal(t, "my/object.txt", key)
}

func TestPresign_Success_WithFakeMinio(t *testing.T) {
	f := &FileClient{bucket: "b", useObject: true, testClient: &fakeMinio{}}
	u, err := f.Presign("my/object.txt", 5*time.Minute)
	require.NoError(t, err)
	require.Contains(t, u, "https://example.com/b/my/object.txt")
}

func TestGet_Success_WithFakeMinio(t *testing.T) {
	f := &FileClient{bucket: "b", useObject: true, testClient: &fakeMinio{}}
	r, err := f.Get("some.txt")
	require.NoError(t, err)
	b, _ := ioutil.ReadAll(r)
	require.Equal(t, "data", string(b))
}

func TestNewFileClientFromEnv_MissingEnv(t *testing.T) {
	// clear envs
	_ = os.Unsetenv("STORAGE_ENDPOINT")
	_ = os.Unsetenv("STORAGE_ACCESS_KEY")
	_ = os.Unsetenv("STORAGE_SECRET_KEY")
	_ = os.Unsetenv("STORAGE_BUCKET")
	_, err := NewFileClientFromEnv()
	require.Error(t, err)
}

func TestNewFileClientFromEnv_MinioNewError(t *testing.T) {
	// provide envs but make minioNew fail
	os.Setenv("STORAGE_ENDPOINT", "ep")
	os.Setenv("STORAGE_ACCESS_KEY", "a")
	os.Setenv("STORAGE_SECRET_KEY", "s")
	os.Setenv("STORAGE_BUCKET", "b")
	prev := minioNew
	minioNew = func(endpoint string, opts *minio.Options) (*minio.Client, error) {
		return nil, fmt.Errorf("boom")
	}
	defer func() { minioNew = prev; _ = os.Unsetenv("STORAGE_ENDPOINT"); _ = os.Unsetenv("STORAGE_ACCESS_KEY"); _ = os.Unsetenv("STORAGE_SECRET_KEY"); _ = os.Unsetenv("STORAGE_BUCKET") }()
	_, err := NewFileClientFromEnv()
	require.Error(t, err)
}

func TestPublicURL_BadBaseURL(t *testing.T) {
	f := &FileClient{bucket: "b", baseURL: "://bad"}
	_, err := f.PublicURL("x")
	require.Error(t, err)
}

func TestSave_Presign_Get_ClientNil(t *testing.T) {
	// When useObject is true but no client/testClient provided, methods should error
	f := &FileClient{bucket: "b", useObject: true, client: nil, testClient: nil}
	_, err := f.Save("k", strings.NewReader("x"))
	require.Error(t, err)
	_, err = f.Presign("k", 5*time.Minute)
	require.Error(t, err)
	_, err = f.Get("k")
	require.Error(t, err)
}
