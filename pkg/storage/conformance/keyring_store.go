package conformance

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/pkp"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/util/future"
)

func pkpKey(pinCount int) *keyring.PKPKey {
	pins := []*pkp.PublicKeyPin{}
	randBytes := make([]byte, 32)
	_, err := io.ReadFull(rand.Reader, randBytes)
	Expect(err).NotTo(HaveOccurred())

	for i := 0; i < pinCount; i++ {
		pins = append(pins, &pkp.PublicKeyPin{
			Algorithm:   lo.Ternary(i%2 == 0, pkp.AlgB2B256, pkp.AlgSHA256),
			Fingerprint: randBytes,
		})
	}
	return keyring.NewPKPKey(pins)
}

func sharedKeys() *keyring.SharedKeys {
	randBytes := make([]byte, 64)
	_, err := io.ReadFull(rand.Reader, randBytes)
	Expect(err).NotTo(HaveOccurred())

	return keyring.NewSharedKeys(randBytes)
}

type testInvalidKeyring struct{}

func (*testInvalidKeyring) Try(...keyring.UseKeyFn) bool {
	return false
}

func (*testInvalidKeyring) ForEach(func(key interface{})) {
}

func (*testInvalidKeyring) Marshal() ([]byte, error) {
	return nil, fmt.Errorf("test error")
}

func (*testInvalidKeyring) Merge(keyring.Keyring) keyring.Keyring {
	return nil
}

func KeyringStoreTestSuite[T storage.KeyringStoreBroker](
	tsF future.Future[T],
) func() {
	return func() {
		var ts storage.KeyringStore
		BeforeAll(func() {
			var err error
			ts, err = tsF.Get().KeyringStore("test", &corev1.Reference{
				Id: "test",
			})
			Expect(err).NotTo(HaveOccurred())
		})
		It("should initially be empty", func() {
			_, err := ts.Get(context.Background())
			Expect(err).To(MatchError(storage.ErrNotFound))
		})
		DescribeTable("Keyring storage",
			func(keys ...any) {
				kr := keyring.New(keys...)
				Eventually(func() error {
					return ts.Put(context.Background(), kr)
				}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
				kr2, err := ts.Get(context.Background())
				Expect(err).NotTo(HaveOccurred())
				Expect(kr2).To(Equal(kr))
			},
			Entry(nil),
			Entry(nil, pkpKey(0)),
			Entry(nil, pkpKey(1)),
			Entry(nil, sharedKeys()),
			Entry(nil, sharedKeys(), pkpKey(0)),
			Entry(nil, sharedKeys(), pkpKey(1)),
			Entry(nil, sharedKeys(), sharedKeys(), pkpKey(1)),
			Entry(nil, sharedKeys(), pkpKey(2)),
			Entry(nil, sharedKeys(), pkpKey(2), pkpKey(3)),
			Entry(nil, sharedKeys(), sharedKeys(), pkpKey(1), pkpKey(1)),
			Entry(nil, sharedKeys(), sharedKeys(), sharedKeys(), sharedKeys()),
			Entry(nil, pkpKey(1), pkpKey(2), pkpKey(3), pkpKey(4)),
		)
		When("putting the keyring into the store", func() {
			It("should error if the keyring is invalid", func() {
				err := ts.Put(context.Background(), &testInvalidKeyring{})
				Expect(err).To(HaveOccurred())
			})
		})
		It("should handle concurrent updates", func() {
			kr := keyring.New(sharedKeys())
			Expect(ts.Put(context.Background(), kr)).To(Succeed())

			kr = keyring.New(sharedKeys(), pkpKey(1))

			var wg sync.WaitGroup
			start := make(chan struct{})
			for i := 0; i < testutil.IfCI(5).Else(10); i++ {
				wg.Add(1)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					<-start
					Expect(ts.Put(context.Background(), kr)).To(Succeed())
				}()
			}
			close(start)
			wg.Wait()

			kr2, err := ts.Get(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(kr2).To(Equal(kr))
		})
		It("should delete keyrings", func() {
			kr := keyring.New(sharedKeys())
			Expect(ts.Put(context.Background(), kr)).To(Succeed())
			_, err := ts.Get(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(ts.Delete(context.Background())).To(Succeed())
			_, err = ts.Get(context.Background())
			Expect(err).To(MatchError(storage.ErrNotFound))
		})
	}
}
