package etcd

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/rancher/opni/pkg/logger"
	"go.uber.org/zap"

	opnisnapshot "github.com/rancher/opni/pkg/snapshot"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/etcdutl/v3/snapshot"
	"go.etcd.io/etcd/server/v3/datadir"
)

const (
	defaultName                     = "default"
	defaultInitialAdvertisePeerURLs = "http://localhost:2380"
)

type Snapshotter struct {
	lg *slog.Logger

	clientConfig *clientv3.Config
	client       *clientv3.Client

	dataDir string
}

func NewSnapshotter(
	clientConfig *clientv3.Config,
	client *clientv3.Client,
	dataDir string,
	lg *slog.Logger,
) *Snapshotter {
	return &Snapshotter{
		lg:           lg,
		clientConfig: clientConfig,
		client:       client,
		dataDir:      dataDir,
	}
}

var _ opnisnapshot.BackupRestore = (*Snapshotter)(nil)

func (s *Snapshotter) Save(ctx context.Context, path string) error {
	endpoint := s.clientConfig.Endpoints[0]
	status, err := s.client.Status(ctx, endpoint)
	if err != nil {
		return err
	}
	if status.IsLearner {
		s.lg.Warn("Unable to take snapshot : not supported for learner")
		return nil
	}

	if err := snapshot.NewV3(zap.NewNop()).Save(
		ctx, *s.clientConfig, path,
	); err != nil {
		s.lg.With(logger.Err(err)).Error("failed to take etcd snapshot")
		return err
	}

	return nil
}

func (s *Snapshotter) Restore(ctx context.Context, path string) error {
	return s.restoreFunc(
		path,
		//FIXME: the following are auto assigned defaults
		//TODO : add link to reference
		initialClusterFromName(defaultName),
		"etcd-cluster",
		"",
		"",
		defaultInitialAdvertisePeerURLs,
		defaultName,
		false,
	)
}
func (s *Snapshotter) restoreFunc(
	snapshotPath string,
	// other thingies
	restoreCluster string,
	restoreClusterToken string,
	restoreDataDir string,
	restoreWalDir string,
	restorePeerURLs string,
	restoreName string,
	skipHashCheck bool,
) error {
	// TODO : this kinda janky
	sp := snapshot.NewV3(zap.NewNop())

	dataDir := restoreDataDir
	if dataDir == "" {
		dataDir = "restore.etcd"
		// dataDir = filepath.Join(s.dataDir, "..", restoreName+".etcd")
	}

	walDir := restoreWalDir
	if walDir == "" {
		walDir = datadir.ToWalDir(dataDir)
	}

	if err := sp.Restore(snapshot.RestoreConfig{
		SnapshotPath:        snapshotPath,
		Name:                restoreName,
		OutputDataDir:       dataDir,
		OutputWALDir:        walDir,
		PeerURLs:            strings.Split(restorePeerURLs, ","),
		InitialCluster:      restoreCluster,
		InitialClusterToken: restoreClusterToken,
		SkipHashCheck:       skipHashCheck,
	}); err != nil {
		return err
	}
	return nil
}

func initialClusterFromName(name string) string {
	n := name
	if name == "" {
		n = defaultName
	}
	return fmt.Sprintf("%s=http://localhost:2380", n)
}
