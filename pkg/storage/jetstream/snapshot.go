// Implementation references the nats CLI implementation :
// https://github.com/nats-io/natscli/blob/875003bdaf4b55a263b1221cf1a64d1b9d482412/cli/account_command.go#L66-L76
package jetstream

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"

	"github.com/dustin/go-humanize"
	"github.com/nats-io/jsm.go"
	"github.com/nats-io/jsm.go/api"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/snapshot"
)

type Snapshotter struct {
	mgr *jsm.Manager

	lg *slog.Logger
}

var _ snapshot.BackupRestore = (*Snapshotter)(nil)

func NewSnapshotter(
	mgr *jsm.Manager,
	lg *slog.Logger,
) *Snapshotter {
	return &Snapshotter{
		mgr: mgr,
		lg:  lg,
	}
}

func (s *Snapshotter) Save(ctx context.Context, path string) error {
	streams, missing, err := s.mgr.Streams(nil)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return fmt.Errorf("could not obtain stream information for %d streams", len(missing))
	}

	if len(streams) == 0 {
		return fmt.Errorf("no streams found")
	}

	totalSize := uint64(0)
	totalConsumers := 0
	for _, s := range streams {
		state, _ := s.LatestState()
		totalConsumers += state.Consumers
		totalSize += state.Bytes
	}

	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}

	for _, stream := range streams {
		where := filepath.Join(path, stream.Name())
		err := s.backupStream(ctx, stream, where)
		if errors.Is(err, jsm.ErrMemoryStreamNotSupported) {
			s.lg.With(logger.Err(err)).Warn(fmt.Sprintf("backup of %s failed", stream.Name()))
		} else if err != nil {
			return fmt.Errorf("backup failed : %w", err)
		}
	}
	return nil
}

func (s *Snapshotter) backupStream(ctx context.Context, stream *jsm.Stream, outputPath string) error {
	fp, err := stream.SnapshotToDirectory(ctx, outputPath, jsm.SnapshotHealthCheck())
	if err != nil {
		return err
	}

	s.lg.Info(
		fmt.Sprintf(
			"Received %s compressed data in %d chunks for stream %q in %v, %s uncompressed \n",
			humanize.IBytes(fp.BytesReceived()),
			fp.ChunksReceived(),
			stream.Name(),
			fp.EndTime().Sub(fp.StartTime()).Round(time.Millisecond),
			humanize.IBytes(fp.UncompressedBytesReceived()),
		),
	)

	return nil
}

func (s *Snapshotter) Restore(ctx context.Context, path string) error {
	streams, err := s.mgr.StreamNames(nil)
	if err != nil {
		return err
	}
	existingStreams := map[string]struct{}{}
	for _, n := range streams {
		existingStreams[n] = struct{}{}
	}
	de, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, d := range de {
		if !d.IsDir() {
			return fmt.Errorf("expected a directory")
		}
		if _, ok := existingStreams[d.Name()]; ok {
			return fmt.Errorf("stream already exists")
		}
		if _, err := os.Stat(filepath.Join(path, d.Name(), "backup.json")); err != nil {
			return errors.Wrap(err, "expected backup.json")
		}
	}
	s.lg.Info(fmt.Sprintf("restoring backup of all %d streams in directory : %q", len(de), path))
	for _, d := range de {
		restorePath := filepath.Join(path, d.Name())
		if err := s.restoreStream(ctx, restorePath); err != nil {
			return err
		}
	}
	return nil
}

func (s *Snapshotter) restoreStream(ctx context.Context, path string) error {
	var bm api.JSApiStreamRestoreRequest
	bmj, err := os.ReadFile(filepath.Join(path, "backup.json"))
	if err != nil {
		return errors.Wrap(err, "restore failed : backup.json not found")
	}
	if err := json.Unmarshal(bmj, &bm); err != nil {
		return errors.Wrap(err, "restore failed")
	}

	known, err := s.mgr.IsKnownStream(bm.Config.Name)
	if err != nil {
		return errors.Wrap(err, "could not check if the stream already exists")
	}
	if known {
		return errors.Wrap(err, fmt.Sprintf("Stream %q already exist", bm.Config.Name))
	}

	opts := []jsm.SnapshotOption{
		jsm.SnapshotDebug(),
		jsm.RestoreConfiguration(bm.Config),
	}

	fp, _, err := s.mgr.RestoreSnapshotFromDirectory(ctx, bm.Config.Name, path, opts...)
	if err != nil {
		return errors.Wrap(err, "restore failed")
	}
	s.lg.Info(fmt.Sprintf(
		"Restored stream %q in %v\n",
		bm.Config.Name,
		fp.EndTime().Sub(fp.StartTime()).Round(time.Second),
	))

	if _, err := s.mgr.LoadStream(bm.Config.Name); err != nil {
		return err
	}
	return nil
}
