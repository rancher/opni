//go:build amd64 && !race

package challenges_test

import (
	"context"
	"path"
	"runtime"
	"runtime/debug"
	"testing"

	"github.com/dterei/gotsc"
	gsync "github.com/kralicky/gpkg/sync"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/challenges"
	"github.com/rancher/opni/pkg/ecdh"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage"
)

// Can't use the test keyring store broker due to possible extra overhead
// from the mock controller and/or unrelated implementation details that
// are otherwise not relevant to tests but may affect timing or performance.
type keyringStoreBroker struct {
	store *gsync.Map[string, keyring.Keyring]
}

func (b *keyringStoreBroker) KeyringStore(prefix string, ref *corev1.Reference) storage.KeyringStore {
	return &keyringStore{
		store:  b.store,
		ref:    ref,
		prefix: prefix,
	}
}

type keyringStore struct {
	store  *gsync.Map[string, keyring.Keyring]
	ref    *corev1.Reference
	prefix string
}

func (ks *keyringStore) Put(_ context.Context, keyring keyring.Keyring) error {
	ks.store.Store(path.Join(ks.prefix, "keyrings", ks.ref.Id), keyring)
	return nil
}

func (ks *keyringStore) Get(_ context.Context) (keyring.Keyring, error) {
	value, ok := ks.store.Load(path.Join(ks.prefix, "keyrings", ks.ref.Id))
	if !ok {
		return nil, storage.ErrNotFound
	}
	return value, nil
}

func (ks *keyringStore) Delete(_ context.Context) error {
	_, ok := ks.store.LoadAndDelete(path.Join(ks.prefix, "keyrings", ks.ref.Id))
	if !ok {
		return storage.ErrNotFound
	}
	return nil
}

var _ = Describe("Keyring Verifier Timing", Ordered, Serial, FlakeAttempts(2), Label("unit", "temporal"), func() {
	BeforeAll(func() {
		if testing.CoverMode() != "" {
			Skip("skipping test when coverage mode is enabled")
		}
		gcPercent := debug.SetGCPercent(-1)
		DeferCleanup(func() {
			debug.SetGCPercent(gcPercent)
		})
	})

	It("should verify challenges in constant time", func() {
		kp1 := ecdh.NewEphemeralKeyPair()
		kp2 := ecdh.NewEphemeralKeyPair()
		sec, err := ecdh.DeriveSharedSecret(kp1, ecdh.PeerPublicKey{
			PublicKey: kp2.PublicKey,
			PeerType:  ecdh.PeerTypeClient,
		})
		Expect(err).NotTo(HaveOccurred())

		testSharedKeys := keyring.NewSharedKeys(sec)
		// testServerKey := testSharedKeys.ServerKey
		testClientKey := testSharedKeys.ClientKey
		testSharedSecret := sec

		broker := &keyringStoreBroker{
			store: &gsync.Map[string, keyring.Keyring]{},
		}
		broker.KeyringStore("gateway", &corev1.Reference{
			Id: "cluster-1",
		}).Put(context.Background(), keyring.New(keyring.NewSharedKeys(testSharedSecret)))
		const domain = "Verifier Timing Test"

		mw := challenges.NewKeyringVerifier(broker, domain, logger.NewNop())

		challenge := [32]byte{
			0xDE, 0xAD, 0xBE, 0xEF, 0xDE, 0xAD, 0xBE, 0xEF,
			0xDE, 0xAD, 0xBE, 0xEF, 0xDE, 0xAD, 0xBE, 0xEF,
			0xDE, 0xAD, 0xBE, 0xEF, 0xDE, 0xAD, 0xBE, 0xEF,
			0xDE, 0xAD, 0xBE, 0xEF, 0xDE, 0xAD, 0xBE, 0xEF,
		}
		cluster1Req := &corev1.ChallengeRequest{
			Challenge: challenge[:],
		}

		clientMeta := challenges.ClientMetadata{
			IdAssertion: "cluster-1",
			Random:      []byte("nothing-up-my-sleeve"),
		}

		response := challenges.Solve(cluster1Req, clientMeta, testClientKey, domain)

		cluster1Verifier, err := mw.Prepare(context.Background(), clientMeta, cluster1Req)
		Expect(err).NotTo(HaveOccurred())

		// sanity-check
		if keys := cluster1Verifier.Verify(response); keys == nil {
			Fail("keys should not be nil")
		}
		response.Response[0] ^= 0x01
		if keys := cluster1Verifier.Verify(response); keys != nil {
			Fail("keys should be nil")
		}
		response.Response[0] ^= 0x01
		if keys := cluster1Verifier.Verify(response); keys == nil {
			Fail("keys should not be nil")
		}

		tsc := gotsc.TSCOverhead()

		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		// for maximum accuracy, switch back and forth between valid and invalid responses.
		// this should limit the impact of variable core frequency and other random factors.
		const n = 20_000_000
		const gcInterval = 100_000
		counters := []uint64{0, 0} // valid cycles, invalid cycles
		counterIdx := 0
		errCount := 0
		var total uint64
		for i := 0; i < n/gcInterval; i++ {
			runtime.GC()
			for ii := 0; ii < gcInterval; ii++ {
				start := gotsc.BenchStart()
				keys := cluster1Verifier.Verify(response)
				end := gotsc.BenchEnd()

				// if the response doesn't match, increment the error counter
				// putting Expect() here does weird things to the benchmark
				// so check once at the end instead
				if keys != nil {
					errCount += counterIdx
				} else {
					errCount += counterIdx ^ 0x01
				}

				// add the cycles to the appropriate counter
				counters[counterIdx] += end - start - tsc
				// flip the first bit of the response to make it invalid (or back to valid)
				response.Response[0] ^= 0x01
				// flip the counter index
				counterIdx ^= 0x01
				total++
			}
		}

		Expect(errCount).To(Equal(0))

		avgCyclesValid := float64(counters[0]) / float64(n/2)
		avgCyclesInvalid := float64(counters[1]) / float64(n/2)

		AddReportEntry("total ops:               ", total)
		AddReportEntry("cycles/op (valid case):  ", avgCyclesValid)
		AddReportEntry("cycles/op (invalid case):", avgCyclesInvalid)

		const threshold = 1 // cycles
		// ensure both cases are approximately equal within a small margin
		Expect(avgCyclesValid).To(BeNumerically("~", avgCyclesInvalid, threshold))
	})
})
