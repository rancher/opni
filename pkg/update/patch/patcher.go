package patch

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/gabstv/go-bsdiff/pkg/bsdiff"
	"github.com/gabstv/go-bsdiff/pkg/bspatch"
	"github.com/klauspost/compress/zstd"
	"github.com/rancher/opni/pkg/config/reactive"
	configv1 "github.com/rancher/opni/pkg/config/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const (
	UpdateStrategy = "binary"
)

type BinaryPatcher interface {
	GeneratePatch(old io.Reader, new io.Reader, patchOut io.Writer) error
	ApplyPatch(old io.Reader, patch io.Reader, newOut io.Writer) error
	CheckFormat(reader io.ReaderAt) bool
}

type BsdiffPatcher struct{}

func (BsdiffPatcher) GeneratePatch(old io.Reader, new io.Reader, patchOut io.Writer) (err error) {
	return bsdiff.Reader(old, new, patchOut)
}

func (BsdiffPatcher) ApplyPatch(old io.Reader, patch io.Reader, newOut io.Writer) (err error) {
	return bspatch.Reader(old, newOut, patch)
}

func (BsdiffPatcher) CheckFormat(reader io.ReaderAt) bool {
	header := []byte("BSDIFF40")
	buf := make([]byte, len(header))
	if _, err := reader.ReadAt(buf, 0); err == nil {
		return bytes.Equal(buf, header)
	}
	return false
}

var allPatchEngines = []BinaryPatcher{
	BsdiffPatcher{},
	ZstdPatcher{},
}

func NewPatcherFromFormat(reader io.ReaderAt) (BinaryPatcher, bool) {
	for _, p := range allPatchEngines {
		if p.CheckFormat(reader) {
			return p, true
		}
	}
	return nil, false
}

type ZstdPatcher struct{}

func (ZstdPatcher) GeneratePatch(old io.Reader, new io.Reader, patchOut io.Writer) (err error) {
	dict, err := io.ReadAll(old)
	if err != nil {
		return err
	}
	enc, err := zstd.NewWriter(nil,
		zstd.WithEncoderDictRaw(0, dict),
		zstd.WithWindowSize(zstd.MaxWindowSize),
		zstd.WithEncoderLevel(zstd.SpeedBestCompression),
	)
	if err != nil {
		return err
	}
	defer enc.Close()
	newBytes, err := io.ReadAll(new)
	if err != nil {
		return err
	}
	patch := enc.EncodeAll(newBytes, nil)
	patchOut.Write(patch)
	return nil
}

func (ZstdPatcher) ApplyPatch(old io.Reader, patch io.Reader, newOut io.Writer) (err error) {
	dict, err := io.ReadAll(old)
	if err != nil {
		return err
	}
	patchBytes, err := io.ReadAll(patch)
	if err != nil {
		return err
	}

	dec, err := zstd.NewReader(nil,
		zstd.WithDecoderDictRaw(0, dict),
		zstd.WithDecoderLowmem(true),
	)
	if err != nil {
		return err
	}
	defer dec.Close()
	out, err := dec.DecodeAll(patchBytes, nil)
	if err != nil {
		return err
	}
	newOut.Write(out)
	return nil
}

func (ZstdPatcher) CheckFormat(reader io.ReaderAt) bool {
	header := "\x28\xb5\x2f\xfd"
	buf := make([]byte, len(header))
	if _, err := reader.ReadAt(buf, 0); err == nil {
		return bytes.Equal(buf, []byte(header))
	}
	return false
}

func NewReactivePatchEngine(ctx context.Context, lg *slog.Logger, config reactive.Value) BinaryPatcher {
	r := &reactivePatcher{}
	var waitOnce sync.Once
	wait := make(chan struct{})
	config.WatchFunc(ctx, func(v protoreflect.Value) {
		r.mu.Lock()
		defer r.mu.Unlock()
		switch v.Enum() {
		case configv1.PatchEngine_Bsdiff.Number():
			r.patcher = BsdiffPatcher{}
			lg.With("engine", "bsdiff").Info("patch engine configured")
		case configv1.PatchEngine_Zstd.Number():
			r.patcher = ZstdPatcher{}
			lg.With("engine", "zstd").Info("patch engine configured")
		default:
			lg.With("engine", v.String()).Warn("unknown patch engine")
		}
		waitOnce.Do(func() {
			close(wait)
		})
	})
	<-wait
	return r
}

type reactivePatcher struct {
	mu      sync.Mutex
	patcher BinaryPatcher
}

func (r *reactivePatcher) GeneratePatch(old io.Reader, new io.Reader, patchOut io.Writer) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.patcher == nil {
		return fmt.Errorf("no patch engine configured")
	}
	return r.patcher.GeneratePatch(old, new, patchOut)
}

func (r *reactivePatcher) ApplyPatch(old io.Reader, patch io.Reader, newOut io.Writer) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.patcher == nil {
		return fmt.Errorf("no patch engine configured")
	}
	return r.patcher.ApplyPatch(old, patch, newOut)
}

func (r *reactivePatcher) CheckFormat(reader io.ReaderAt) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.patcher == nil {
		return false
	}
	return r.patcher.CheckFormat(reader)
}
