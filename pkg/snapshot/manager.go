package snapshot

import (
	"archive/zip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/rancher/opni/pkg/validation"
	"github.com/samber/lo"

	"emperror.dev/errors"
	"github.com/rancher/opni/pkg/logger"

	// TODO : one or more inflight transactions
	"golang.org/x/sync/semaphore"
)

const (
	maxConcurrentSnapshots = 1
	compressedExtension    = ".zip"
	_metadataDir           = ".metadata"
)

const (
	LocationPrefixLocal = "file://"
	LocationPrefixS3    = "s3://"
)

const (
	SnapshotStatusSuccessful SnapshotStatus = "successful"
	SnapshotStatusFailed     SnapshotStatus = "failed"
)

const (
	CompressionZip CompressionType = "zip"
)

type SnapshotStatus string

type CompressionType string

type SnapshotConfig struct {
	SnapshotDir  string
	SnapshotName string

	DataDir string
	// 0 indicates no retention limit is enforced
	Retention int

	Compression *CompressionConfig
	S3          *S3Config
}

func (c *SnapshotConfig) Validate() error {
	if c.SnapshotName == "" {
		return validation.Error("snapshot name required")
	}
	if c.DataDir == "" {
		return validation.Error("data dir required")
	}
	return nil
}

func (c *SnapshotConfig) compressionEnabled() bool {
	return c.Compression != nil
}

func (c *SnapshotConfig) s3Enabled() bool {
	return c.S3 != nil
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
	Type CompressionType
}

type Snapshotter interface {
	Save(ctx context.Context, snapshotPath string) error
}

type Restorer interface {
	Restore(ctx context.Context, path string) error
}

type BackupRestore interface {
	Snapshotter
	Restorer
}

type SnapshotManager interface {
	Save(ctx context.Context) error
	List(ctx context.Context) ([]SnapshotMetadata, error)
	Restore(ctx context.Context, snapMd SnapshotMetadata) error
}

type snapshotManager struct {
	lg     *slog.Logger
	config SnapshotConfig
	impl   BackupRestore

	s3Client  *minio.Client
	semaphore *semaphore.Weighted
}

var _ SnapshotManager = (*snapshotManager)(nil)

func NewSnapshotManager(
	impl BackupRestore,
	config SnapshotConfig,
	lg *slog.Logger,
) SnapshotManager {
	return &snapshotManager{
		impl:      impl,
		config:    config,
		lg:        lg,
		semaphore: semaphore.NewWeighted(1),
	}
}

// TODO : we may not need the default snapshot dir behaviour
func (s *snapshotManager) snapshotDir(create bool) (string, error) {
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

func (s *snapshotManager) metadataDir(create bool) (string, error) {
	snapDir, err := s.snapshotDir(create)
	if err != nil {
		return snapDir, err
	}

	metadataDir := filepath.Join(snapDir, _metadataDir)
	_, err = os.Stat(metadataDir)
	if err != nil {
		if create && os.IsNotExist(err) {
			if err := os.MkdirAll(metadataDir, 0700); err != nil {
				return "", err
			}
		}
	}

	return metadataDir, nil
}

func (s *snapshotManager) commitMetadata(sf *SnapshotMetadata) error {
	metadataDir, err := s.metadataDir(true)
	if err != nil {
		return err
	}
	mdPath := filepath.Join(metadataDir, sf.Name)
	data, err := json.Marshal(sf)
	if err != nil {
		return err
	}
	s.lg.With("path", mdPath).Info("commiting metadata")
	return os.WriteFile(mdPath, data, 0700)
}

func (s *snapshotManager) List(ctx context.Context) ([]SnapshotMetadata, error) {
	metadataDir, err := s.metadataDir(false)
	if err != nil {
		// TODO : maybe do some error handling here and return appropriate status codes
		return []SnapshotMetadata{}, err
	}
	s.lg.With("path", metadataDir).Info("listing local metadata")
	fSys := os.DirFS(metadataDir)
	res := []SnapshotMetadata{}
	err = fs.WalkDir(fSys, ".", func(path string, d fs.DirEntry, _ error) error {
		s.lg.With("path", path).Info("checking")
		if path == "" {
			return nil
		}
		if d != nil && !d.IsDir() {
			data, err := os.ReadFile(filepath.Join(metadataDir, path))
			if err != nil {
				s.lg.With(logger.Err(err)).Error("failed to read snapshot metadata contents")
				return err
			}
			var sf SnapshotMetadata
			if err := json.Unmarshal(data, &sf); err != nil {
				s.lg.With(logger.Err(err)).Warn("failed to unmarshal snapshot file contents, skipping")
			} else {
				res = append(res, sf)
			}
		}
		return nil
	})
	// TODO : don't necessarily return error here
	if err != nil {
		return nil, err
	}

	// TODO : check S3 data here if config is defined
	return res, nil
}

func (s *snapshotManager) compressionConfig() *compressionMetadata {
	return lo.Ternary[*compressionMetadata](
		s.config.compressionEnabled(),
		&compressionMetadata{
			Type: CompressionZip,
		},
		nil,
	)
}

func (s *snapshotManager) Save(ctx context.Context) error {
	if ack := s.semaphore.TryAcquire(1); !ack {
		return errors.New("snapshot already in progress")
	}
	now := time.Now().Round(time.Second)
	snapshotName := fmt.Sprintf(
		"%s-opni-%d",
		s.config.SnapshotName,
		now.Unix(),
	)
	snapshotDir, err := s.snapshotDir(true)
	if err != nil {
		return err
	}
	snapshotPath := filepath.Join(snapshotDir, snapshotName)
	lg := s.lg.With("snapshot", snapshotName, "path", snapshotPath)
	var sf *SnapshotMetadata
	if err := s.impl.Save(ctx, snapshotPath); err != nil {
		sf = &SnapshotMetadata{
			Name:      snapshotName,
			Location:  "",
			CreatedAt: now,
			Status:    SnapshotStatusFailed,
			Message:   base64.StdEncoding.EncodeToString([]byte(err.Error())),
			Size:      0,
		}

		if err := s.commitMetadata(sf); err != nil {
			lg.With(logger.Err(err)).Error("failed to commit metadata after failed snapshot")
			return err
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
			lg.Info("compressed snapshot")
		}
		f, err := os.Stat(snapshotPath)
		if err != nil {
			return errors.Wrap(err, "unable to retrieve snapshot information from local snapshot")
		}
		sf = &SnapshotMetadata{
			Name:                f.Name(),
			Location:            LocationPrefixLocal + snapshotPath,
			CreatedAt:           now,
			Status:              SnapshotStatusSuccessful,
			Size:                f.Size(),
			Compressed:          s.config.compressionEnabled(),
			CompressionMetadata: s.compressionConfig(),
		}

		if err := s.commitMetadata(sf); err != nil {
			lg.Warn("failed to commit metadata after successful snapshot")
		}

		// TODO : check/prune based on retention limits
		if err := s.retention(s.config.Retention, s.config.SnapshotName, s.config.SnapshotDir); err != nil {
			return errors.Wrap(err, "failed to apply local snapshot retention policy")
		}

		if s.config.s3Enabled() {
			if err := s.preRunS3(); err != nil {
				sf = &SnapshotMetadata{
					Name:      filepath.Base(snapshotPath),
					CreatedAt: now,
					Message:   base64.StdEncoding.EncodeToString([]byte(err.Error())),
					Size:      0,
					Status:    SnapshotStatusFailed,
					S3: &s3Metadata{
						Endpoint:      s.config.S3.Endpoint,
						EndpointCA:    s.config.S3.EndpointCA,
						SkipSSLVerify: s.config.S3.SkipSSLVerify,
						Bucket:        s.config.S3.BucketName,
						Region:        s.config.S3.Region,
						Folder:        s.config.S3.Folder,
						Insecure:      s.config.S3.Insecure,
					},
				}
			} else {
				// if init succeeds here, try upload
				sf, err = s.uploadS3(ctx, snapshotPath, now)
				if err != nil {
					lg.With(logger.Err(err)).Error(
						"Error received during snapshot upload %s", err)
				} else {
					lg.Info("S3 upload complete")
				}

				if err := s.retentionS3(); err != nil {
					lg.With(logger.Err(err)).Error(
						"failed to apply s3 snapshot retention policy",
					)
				}
			}
		}
		if err := s.commitMetadata(sf); err != nil {
			lg.Warn("failed to persist metadata locally")
		}
	}
	return nil
}

func (s *snapshotManager) preRunS3() error {
	if s.s3Client == nil {
		client, err := s.initS3Client()
		if err != nil {
			return err
		}
		s.s3Client = client
	}
	return nil
}

// snapshotDir ensures that the snapshot directory exists, and then returns its path.

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

func (s *snapshotManager) initS3Client() (*minio.Client, error) {
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

func (s *snapshotManager) uploadS3(ctx context.Context, snapshotPath string, now time.Time) (*SnapshotMetadata, error) {
	s.lg.Info(fmt.Sprintf("Uploading snapshot to s3://%s/%s", s.config.S3.BucketName, snapshotPath))

	basename := filepath.Base(snapshotPath)
	metadata := filepath.Join(filepath.Dir(snapshotPath), "..", _metadataDir, basename)
	snapshotKey := path.Join(s.config.S3.Folder, basename)
	metadataKey := path.Join(s.config.S3.Folder, _metadataDir, basename)

	sf := &SnapshotMetadata{
		Name:      basename,
		Location:  LocationPrefixS3 + fmt.Sprintf("%s/%s", s.config.S3.BucketName, snapshotKey),
		CreatedAt: now,
		S3: &s3Metadata{
			Endpoint:      s.config.S3.Endpoint,
			EndpointCA:    s.config.S3.EndpointCA,
			SkipSSLVerify: s.config.S3.SkipSSLVerify,
			Bucket:        s.config.S3.BucketName,
			Region:        s.config.S3.Region,
			Folder:        s.config.S3.Folder,
			Insecure:      s.config.S3.Insecure,
		},
		Compressed:          strings.HasSuffix(snapshotPath, compressedExtension),
		CompressionMetadata: s.compressionConfig(),
	}

	client, err := s.initS3Client()
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize s3 client")
	}

	uploadInfo, err := s.uploadS3Snapshot(client, ctx, snapshotKey, snapshotPath)
	if err != nil {
		sf.Status = SnapshotStatusFailed
		sf.Message = base64.StdEncoding.EncodeToString([]byte(err.Error()))
	} else {
		sf.Status = SnapshotStatusSuccessful
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

func (s *snapshotManager) uploadS3Snapshot(
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

func (s *snapshotManager) uploadS3Metadata(
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

func (s *snapshotManager) retention(retention int, snapshotPrefix string, snapshotDir string) error {
	// TODO
	return nil
}

func (s *snapshotManager) retentionS3() error {
	// TODO
	return nil
}

func (s *snapshotManager) compressSnapshot(
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

func (s *snapshotManager) Restore(ctx context.Context, snapMd SnapshotMetadata) error {
	if err := snapMd.Validate(); err != nil {
		return err
	}
	switch {
	case strings.HasPrefix(snapMd.Location, LocationPrefixLocal):
		return s.restoreFromFile(ctx, snapMd)
	case strings.HasPrefix(snapMd.Location, LocationPrefixS3):
		return s.restoreFromS3(ctx, snapMd)
	default:
		return errors.New(
			fmt.Sprintf(
				"unimplemented encoded storage type in '%s' for restore",
				snapMd.Location,
			))
	}
}

func (s *snapshotManager) restoreFromFile(ctx context.Context, snapMd SnapshotMetadata) error {
	path := strings.TrimPrefix(snapMd.Location, LocationPrefixLocal)

	if snapMd.Compressed {
		var err error
		path, err = s.decompress(path)
		if err != nil {
			return err
		}
	}

	return s.impl.Restore(ctx, path)

}

func (s *snapshotManager) restoreFromS3(ctx context.Context, snapMd SnapshotMetadata) error {
	if !s.config.s3Enabled() {
		return errors.New("s3 is not enabled in snapshot manager despite having an s3 snapshot")
	}
	client, err := s.initS3Client()
	if err != nil {
		return err
	}
	opts := minio.GetObjectOptions{}
	exists, err := client.BucketExists(ctx, s.config.S3.BucketName)
	if err != nil {
		return errors.Wrap(err, "failed to verify bucket existence")
	}
	if !exists {
		return fmt.Errorf("bucket '%s' does not exist", s.config.S3.BucketName)
	}
	path := strings.TrimPrefix(snapMd.Location, LocationPrefixS3)

	if err := client.FGetObject(ctx, s.config.S3.BucketName, path, path, opts); err != nil {
		return err
	}

	if snapMd.Compressed {
		var err error
		path, err = s.decompress(path)
		if err != nil {
			return errors.Wrap(err, "failed to decompress downloaded S3 snapshot")
		}
	}

	return s.impl.Restore(ctx, path)
}

func (s *snapshotManager) decompress(path string) (string, error) {
	zf, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer zf.Close()

	if len(zf.File) != 1 {
		return "", errors.New("expected only one file")
	}
	fileToExtract := zf.File[0]
	fileReader, err := fileToExtract.Open()
	if err != nil {
		return "", err
	}
	defer fileReader.Close()

	extractedFileP := strings.TrimSuffix(path, string(CompressionZip))
	extractedFile, err := os.Create(extractedFileP)
	if err != nil {
		s.lg.Error(err.Error())
	}
	defer extractedFile.Close()

	if _, err := io.Copy(extractedFile, fileReader); err != nil {
		return "", err
	}
	return extractedFileP, nil
}

// SnapshotMetadata represents a single snapshot and its metadata.
type SnapshotMetadata struct {
	Name string `json:"name"`
	// Location contains the full path of the snapshot. For
	// local paths, the location will be prefixed with "file://".
	Location            string               `json:"location,omitempty"`
	Metadata            string               `json:"metadata,omitempty"`
	Message             string               `json:"message,omitempty"`
	CreatedAt           time.Time            `json:"createdAt,omitempty"`
	Size                int64                `json:"size,omitempty"`
	Status              SnapshotStatus       `json:"status,omitempty"`
	S3                  *s3Metadata          `json:"s3Metadata,omitempty"`
	Compressed          bool                 `json:"compressed"`
	CompressionMetadata *compressionMetadata `json:"compressionMetadata"`
}

type s3Metadata struct {
	Endpoint      string `json:"endpoint,omitempty"`
	EndpointCA    string `json:"endpointCA,omitempty"`
	SkipSSLVerify bool   `json:"skipSSLVerify,omitempty"`
	Bucket        string `json:"bucket,omitempty"`
	Region        string `json:"region,omitempty"`
	Folder        string `json:"folder,omitempty"`
	Insecure      bool   `json:"insecure,omitempty"`
}

type compressionMetadata struct {
	Type CompressionType `json:"type"`
}

func (c *SnapshotMetadata) Validate() error {
	if c.Location == "" {
		return errors.New("snapshot location required")
	}
	if c.Compressed && c.CompressionMetadata == nil {
		return errors.New("mismatched compression fields")
	}
	return nil
}
