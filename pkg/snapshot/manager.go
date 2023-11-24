package snapshot

import (
	"archive/zip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"emperror.dev/errors"
	"github.com/rancher/opni/pkg/logger"
)

const (
	maxConcurrentSnapshots = 1
	compressedExtension    = ".zip"
	metadataDir            = ".metadata"
)

const (
	successfulSnapshotStatus SnapshotStatus = "successful"
	failedSnapshotStatus     SnapshotStatus = "failed"
)

type SnapshotStatus string

type SnapshotConfig struct {
	SnapshotDir  string
	SnapshotName string

	DataDir string

	Retention int

	Compression *CompressionConfig
	S3          *S3Config
}

func (s *SnapshotConfig) compressionEnabled() bool {
	return s.Compression != nil
}

func (s *SnapshotConfig) s3Enabled() bool {
	return s.S3 != nil
}

type S3Config struct {
	Endpoint      string        `json:"-"`
	EndpointCA    string        `json:"-"`
	SkipSSLVerify bool          `json:"-"`
	AccessKey     string        `json:"-"`
	SecretKey     string        `json:"-"`
	BucketName    string        `json:"-"`
	Region        string        `json:"-"`
	Folder        string        `json:"-"`
	Timeout       time.Duration `json:"-"`
	Insecure      bool          `json:"-"`
}

type CompressionConfig struct {
	Type string
}

type AbstractSnapshotter interface {
	Save(ctx context.Context, snapshotPath string) error
}

type AbstractSnapshotManager interface {
	Save(ctx context.Context) error
	List(ctx context.Context) error
}

type SnapshotManager struct {
	lg     *slog.Logger
	config SnapshotConfig
	impl   AbstractSnapshotter
}

var _ AbstractSnapshotManager = (*SnapshotManager)(nil)

func NewSnapshotManager(
	impl AbstractSnapshotter,
) *SnapshotManager {
	return &SnapshotManager{
		impl: impl,
	}
}

func (s *SnapshotManager) List(ctx context.Context) error {
	return nil
}

func (s *SnapshotManager) Save(ctx context.Context) error {
	nodeName := "TODO"
	now := time.Now().Round(time.Second)
	snapshotName := fmt.Sprintf(
		"%s-%s-%d",
		s.config.SnapshotName,
		nodeName,
		now.Unix(),
	)
	snapshotDir, err := s.snapshotDir(true)
	if err != nil {
		return err
	}
	snapshotPath := filepath.Join(snapshotDir, snapshotName)
	var sf *snapshotFile
	if err := s.impl.Save(ctx, snapshotPath); err != nil {
		sf = &snapshotFile{
			Name:      snapshotName,
			Location:  "",
			CreatedAt: now,
			Status:    failedSnapshotStatus,
			Message:   base64.StdEncoding.EncodeToString([]byte(err.Error())),
			Size:      0,
			// metadataSource: extraMetadata,
		}
	}

	if sf == nil {
		if s.config.compressionEnabled() {
			zipPath, err := s.compressSnapshot(
				s.config.SnapshotDir, snapshotName, snapshotPath, now,
			)
			if err != nil {
				return errors.Wrap(err, "failed to compress etcd snapshot")
			}
			snapshotPath = zipPath
			s.lg.Info("compressed snapshot")
		}
		f, err := os.Stat(snapshotPath)
		if err != nil {
			return errors.Wrap(err, "unable to retrieve snapshot information from local snapshot")
		}
		sf = &snapshotFile{
			Name:       f.Name(),
			Location:   "file://" + snapshotPath,
			CreatedAt:  now,
			Status:     successfulSnapshotStatus,
			Size:       f.Size(),
			Compressed: s.config.compressionEnabled(),
		}

		// TODO : persist snapshot metadata file locally

		// TODO : check/prune retention limits
		if err := s.retention(s.config.Retention, s.config.SnapshotName, s.config.SnapshotDir); err != nil {
			return errors.Wrap(err, "failed to apply local snapshot retention policy")
		}

		if s.config.s3Enabled() {
			// TODO : init client here
			sf = &snapshotFile{
				Name:      filepath.Base(snapshotPath),
				CreatedAt: now,
				Message:   base64.StdEncoding.EncodeToString([]byte(err.Error())),
				Size:      0,
				Status:    failedSnapshotStatus,
				S3: &s3Config{
					Endpoint:      s.config.S3.Endpoint,
					EndpointCA:    s.config.S3.EndpointCA,
					SkipSSLVerify: s.config.S3.SkipSSLVerify,
					Bucket:        s.config.S3.BucketName,
					Region:        s.config.S3.Region,
					Folder:        s.config.S3.Folder,
					Insecure:      s.config.S3.Insecure,
				},
			}
			// if init succeeds here, try upload
			sf, err := s.uploadS3(ctx, snapshotPath, now)
			if err != nil {
				s.lg.With(logger.Err(err)).Error(
					"Error received during snapshot upload %s", err)
			} else {
				s.lg.Info("S3 upload complete")
			}

			if err := s.retentionS3(); err != nil {
				s.lg.With(logger.Err(err)).Error(
					"failed to apply s3 snapshot retention policy",
				)
			}

			// TODO : persist metadata file locally
			fmt.Println(sf)
			// either it is snapshot md or s3 failure record
		}
	}
	return nil
}

// snapshotDir ensures that the snapshot directory exists, and then returns its path.
func (s *SnapshotManager) snapshotDir(create bool) (string, error) {
	if s.config.SnapshotDir == "" {
		// we have to create the snapshot dir if we are using
		// the default snapshot dir if it doesn't exist
		defaultSnapshotDir := filepath.Join(s.config.DataDir, "db", "snapshots")
		s, err := os.Stat(defaultSnapshotDir)
		if err != nil {
			if create && os.IsNotExist(err) {
				if err := os.MkdirAll(defaultSnapshotDir, 0700); err != nil {
					return "", err
				}
				return defaultSnapshotDir, nil
			}
			return "", err
		}
		if s.IsDir() {
			return defaultSnapshotDir, nil
		}
	}
	return s.config.SnapshotDir, nil
}

// isValidCertificate checks to see if the given
// byte slice is a valid x509 certificate.
func isValidCertificate(c []byte) bool {
	p, _ := pem.Decode(c)
	if p == nil {
		return false
	}
	if _, err := x509.ParseCertificates(p.Bytes); err != nil {
		return false
	}
	return true
}

func setTransportCA(tr http.RoundTripper, endpointCA string, insecureSkipVerify bool) (http.RoundTripper, error) {
	// TODO : we probably don't need this
	ca, err := readS3EndpointCA(endpointCA)
	if err != nil {
		return tr, err
	}
	if !isValidCertificate(ca) {
		return tr, errors.New("endpoint-ca is not a valid x509 certificate")
	}

	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(ca)

	tr.(*http.Transport).TLSClientConfig = &tls.Config{
		RootCAs:            certPool,
		InsecureSkipVerify: insecureSkipVerify,
	}

	return tr, nil
}

func readS3EndpointCA(endpointCA string) ([]byte, error) {
	ca, err := base64.StdEncoding.DecodeString(endpointCA)
	if err != nil {
		return os.ReadFile(endpointCA)
	}
	return ca, nil
}

func (s *SnapshotManager) initS3Client() (*minio.Client, error) {
	if s.config.S3.BucketName == "" {
		return nil, errors.New("s3 bucket name was not set")
	}
	tr := http.DefaultTransport

	switch {
	case s.config.S3.EndpointCA != "":
		trCA, err := setTransportCA(tr, s.config.S3.EndpointCA, s.config.S3.SkipSSLVerify)
		if err != nil {
			return nil, err
		}
		tr = trCA
	case s.config.S3.SkipSSLVerify:
		tr.(*http.Transport).TLSClientConfig = &tls.Config{
			InsecureSkipVerify: s.config.S3.SkipSSLVerify,
		}
	}

	creds := credentials.NewStaticV4(s.config.S3.AccessKey, s.config.S3.SecretKey, "")
	opt := minio.Options{
		Creds:        creds,
		Secure:       !s.config.S3.Insecure,
		Region:       s.config.S3.Region,
		Transport:    tr,
		BucketLookup: minio.BucketLookupAuto,
	}
	c, err := minio.New(s.config.S3.Endpoint, &opt)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (s *SnapshotManager) uploadS3(ctx context.Context, snapshotPath string, now time.Time) (*snapshotFile, error) {
	s.lg.Info("Uploading snapshot to s3://%s/%s", s.config.S3.BucketName, snapshotPath)

	basename := filepath.Base(snapshotPath)
	metadata := filepath.Join(filepath.Dir(snapshotPath), "..", metadataDir, basename)
	snapshotKey := path.Join(s.config.S3.Folder, basename)
	metadataKey := path.Join(s.config.S3.Folder, metadataDir, basename)

	sf := &snapshotFile{
		Name:      basename,
		Location:  fmt.Sprintf("s3://%s/%s", s.config.S3.BucketName, snapshotKey),
		CreatedAt: now,
		S3: &s3Config{
			Endpoint:      s.config.S3.Endpoint,
			EndpointCA:    s.config.S3.EndpointCA,
			SkipSSLVerify: s.config.S3.SkipSSLVerify,
			Bucket:        s.config.S3.BucketName,
			Region:        s.config.S3.Region,
			Folder:        s.config.S3.Folder,
			Insecure:      s.config.S3.Insecure,
		},
		Compressed: strings.HasSuffix(snapshotPath, compressedExtension),
	}

	client, err := s.initS3Client()
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize s3 client")
	}

	uploadInfo, err := s.uploadS3Snapshot(client, ctx, snapshotKey, snapshotPath)
	if err != nil {
		sf.Status = failedSnapshotStatus
		sf.Message = base64.StdEncoding.EncodeToString([]byte(err.Error()))
	} else {
		sf.Status = successfulSnapshotStatus
		sf.Size = uploadInfo.Size
	}
	if _, err := s.uploadS3Metadata(client, ctx, metadataKey, metadata); err != nil {
		s.lg.With(logger.Err(err)).Warn("failed to upload snapshot metadata to S3")
	} else {
		s.lg.With("bucket", s.config.S3.BucketName, "key", metadataKey).Info(
			"Uploaded snapshot metadata",
		)
	}
	return sf, nil
}

func (s *SnapshotManager) uploadS3Snapshot(
	client *minio.Client,
	ctx context.Context,
	key, path string,
) (minio.UploadInfo, error) {
	opts := minio.PutObjectOptions{
		NumThreads:   2,
		UserMetadata: map[string]string{
			// TODO : put meaningful info here
		},
	}
	if strings.HasSuffix(key, compressedExtension) {
		opts.ContentType = "application/zip"
	} else {
		opts.ContentType = "application/octet-stream"
	}
	ctxca, ca := context.WithTimeout(ctx, s.config.S3.Timeout)
	defer ca()
	return client.FPutObject(ctxca, s.config.S3.BucketName, key, path, opts)
}

func (s *SnapshotManager) uploadS3Metadata(
	client *minio.Client,
	ctx context.Context,
	key, path string,
) (minio.UploadInfo, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return minio.UploadInfo{}, nil
		}
	}

	opts := minio.PutObjectOptions{
		NumThreads:   2,
		ContentType:  "application/json",
		UserMetadata: map[string]string{
			// TODO : meaningful md here
		},
	}
	ctxca, ca := context.WithTimeout(ctx, s.config.S3.Timeout)
	defer ca()
	return client.FPutObject(ctxca, s.config.S3.BucketName, key, path, opts)
}

func (s *SnapshotManager) retention(retention int, snapshotPrefix string, snapshotDir string) error {
	return nil
}

func (s *SnapshotManager) retentionS3() error {
	return nil
}

func (s *SnapshotManager) compressSnapshot(
	snapshotDir, snapshotName, snapshotPath string,
	now time.Time,
) (string, error) {
	s.lg.Info(fmt.Sprintf("Compressing etcd snapshot file : %s", snapshotName))

	zippedSnapshotName := snapshotName + compressedExtension
	zipPath := filepath.Join(snapshotDir, zippedSnapshotName)

	zf, err := os.Create(zipPath)
	if err != nil {
		return "", err
	}
	defer zf.Close()

	zipWriter := zip.NewWriter(zf)
	defer zipWriter.Close()

	uncompressedPath := filepath.Join(snapshotDir, snapshotName)
	fileToZip, err := os.Open(uncompressedPath)
	if err != nil {
		os.Remove(zipPath)
		return "", err
	}
	defer fileToZip.Close()

	info, err := fileToZip.Stat()
	if err != nil {
		os.Remove(zipPath)
		return "", err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		os.Remove(zipPath)
		return "", err
	}

	header.Name = snapshotName
	header.Method = zip.Deflate
	header.Modified = now

	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		os.Remove(zipPath)
		return "", err
	}
	_, err = io.Copy(writer, fileToZip)

	return zipPath, err
}

// snapshotFile represents a single snapshot and it's
// metadata.
type snapshotFile struct {
	Name string `json:"name"`
	// Location contains the full path of the snapshot. For
	// local paths, the location will be prefixed with "file://".
	Location   string         `json:"location,omitempty"`
	Metadata   string         `json:"metadata,omitempty"`
	Message    string         `json:"message,omitempty"`
	CreatedAt  time.Time      `json:"createdAt,omitempty"`
	Size       int64          `json:"size,omitempty"`
	Status     SnapshotStatus `json:"status,omitempty"`
	S3         *s3Config      `json:"s3Config,omitempty"`
	Compressed bool           `json:"compressed"`
}

type s3Config struct {
	Endpoint      string `json:"endpoint,omitempty"`
	EndpointCA    string `json:"endpointCA,omitempty"`
	SkipSSLVerify bool   `json:"skipSSLVerify,omitempty"`
	Bucket        string `json:"bucket,omitempty"`
	Region        string `json:"region,omitempty"`
	Folder        string `json:"folder,omitempty"`
	Insecure      bool   `json:"insecure,omitempty"`
}
