// Package storage wraps any S3-compatible object store: Cloudflare R2
// (recommended: zero egress fees), Backblaze B2, Hetzner Object Storage,
// AWS S3, or MinIO locally. Clients upload/download directly with short-lived
// presigned URLs, so file bytes never pass through (or cost) the API server.
package storage

import (
	"context"
	"io"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"app/internal/config"
)

type Store struct {
	c          *minio.Client
	bucket     string
	publicBase string
}

func New(cfg *config.Config) (*Store, error) {
	c, err := minio.New(cfg.S3Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.S3AccessKey, cfg.S3SecretKey, ""),
		Secure: cfg.S3UseSSL,
		Region: cfg.S3Region,
	})
	if err != nil {
		return nil, err
	}
	return &Store{c: c, bucket: cfg.S3Bucket, publicBase: cfg.S3PublicBaseURL}, nil
}

func (s *Store) PresignPut(ctx context.Context, key string, ttl time.Duration) (string, error) {
	u, err := s.c.PresignedPutObject(ctx, s.bucket, key, ttl)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// URL returns a public CDN URL for public objects, otherwise a presigned GET.
func (s *Store) URL(ctx context.Context, key string, public bool, ttl time.Duration, downloadName string) (string, error) {
	if public && s.publicBase != "" {
		return s.publicBase + "/" + key, nil
	}
	params := url.Values{}
	if downloadName != "" {
		params.Set("response-content-disposition", `attachment; filename="`+sanitize(downloadName)+`"`)
	}
	u, err := s.c.PresignedGetObject(ctx, s.bucket, key, ttl, params)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

type ObjectInfo struct {
	Size        int64
	ContentType string
}

func (s *Store) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	st, err := s.c.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	return ObjectInfo{Size: st.Size, ContentType: st.ContentType}, err
}

func (s *Store) Download(ctx context.Context, key, path string) error {
	return s.c.FGetObject(ctx, s.bucket, key, path, minio.GetObjectOptions{})
}

type PutOpts struct {
	ContentType     string
	ContentEncoding string
	CacheControl    string
}

func (s *Store) UploadFile(ctx context.Context, key, path string, o PutOpts) (int64, error) {
	info, err := s.c.FPutObject(ctx, s.bucket, key, path, minio.PutObjectOptions{
		ContentType: o.ContentType, ContentEncoding: o.ContentEncoding, CacheControl: o.CacheControl,
	})
	return info.Size, err
}

func (s *Store) Put(ctx context.Context, key string, r io.Reader, size int64, o PutOpts) error {
	_, err := s.c.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{ContentType: o.ContentType})
	return err
}

func (s *Store) Delete(ctx context.Context, keys ...string) error {
	for _, k := range keys {
		if err := s.c.RemoveObject(ctx, s.bucket, k, minio.RemoveObjectOptions{}); err != nil {
			return err
		}
	}
	return nil
}

// EnsureBucket creates the bucket in development (MinIO). In production create it in your provider's console.
func (s *Store) EnsureBucket(ctx context.Context) error {
	ok, err := s.c.BucketExists(ctx, s.bucket)
	if err != nil || ok {
		return err
	}
	return s.c.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{})
}

func sanitize(name string) string {
	out := make([]rune, 0, len(name))
	for _, r := range name {
		if r == '"' || r == '\\' || r < 32 || r == 127 {
			continue
		}
		out = append(out, r)
	}
	if len(out) > 150 {
		out = out[:150]
	}
	return string(out)
}
