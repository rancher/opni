package storage_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"os"

	"golang.org/x/sync/errgroup"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/rancher/opni-monitoring/pkg/keyring"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"github.com/rancher/opni-monitoring/pkg/test"
)

const (
	testNamespace = "default"
)

func randomData() []byte {
	buf := new(bytes.Buffer)
	io.CopyN(buf, rand.Reader, 64)
	return buf.Bytes()
}

var _ = Describe("Secret", Ordered, func() {
	var cfg *rest.Config
	var client *kubernetes.Clientset
	BeforeAll(func() {
		env := &test.Environment{
			TestBin: "../../testbin/bin",
			Logger:  logger.New().Named("test"),
		}
		var err error
		cfg, err = env.StartK8s()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(env.Stop)
		client = kubernetes.NewForConfigOrDie(cfg)
	})
	var store *storage.SecretStore
	It("should create a new secret store", func() {
		var err error
		store, err = storage.NewSecretStoreFromConfig(cfg, testNamespace)
		Expect(err).NotTo(HaveOccurred())
		Expect(store).NotTo(BeNil())
	})
	When("no keyring has been created yet", func() {
		It("should return an error", func() {
			_, err := store.Get(context.Background())
			Expect(err).To(MatchError(storage.ErrNotFound))
		})
	})
	When("writing a keyring to the store", func() {
		var kr keyring.Keyring
		It("should create a kubernetes secret with the keyring contents", func() {
			kr = keyring.New(keyring.NewSharedKeys(randomData()))
			err := store.Put(context.Background(), kr)
			Expect(err).NotTo(HaveOccurred())

			sec, err := client.CoreV1().Secrets(testNamespace).Get(context.Background(),
				"opni-tenant-keyring", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			expectedData, err := kr.Marshal()
			Expect(err).NotTo(HaveOccurred())
			Expect(sec.Data).To(HaveKey("keyring"))
			Expect(sec.Data["keyring"]).To(Equal(expectedData))
		})
		It("should retrieve the keyring from the store", func() {
			expected, _ := kr.Marshal()
			stored, err := store.Get(context.Background())
			Expect(err).NotTo(HaveOccurred())
			actual, _ := stored.Marshal()
			Expect(actual).To(Equal(expected))
		})
	})
	When("the keyring secret already exists", func() {
		It("should be overwritten", func() {
			sec, err := client.CoreV1().Secrets(testNamespace).Get(context.Background(),
				"opni-tenant-keyring", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			uid := sec.UID
			resourceVersion := sec.ResourceVersion

			keyring := keyring.New(keyring.NewSharedKeys(randomData()))
			expectedData, err := keyring.Marshal()
			Expect(err).NotTo(HaveOccurred())

			err = store.Put(context.Background(), keyring)
			Expect(err).NotTo(HaveOccurred())

			sec, err = client.CoreV1().Secrets(testNamespace).Get(context.Background(),
				"opni-tenant-keyring", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			Expect(sec.Data).To(HaveKey("keyring"))
			Expect(sec.Data["keyring"]).To(Equal(expectedData))
			Expect(sec.UID).To(Equal(uid))
			Expect(sec.ResourceVersion).NotTo(Equal(resourceVersion))
		})
	})
	When("the secret is deleted", func() {
		It("should return an error when trying to get the keyring", func() {
			err := client.CoreV1().Secrets(testNamespace).Delete(context.Background(),
				"opni-tenant-keyring", metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Get(context.Background())
			Expect(err).To(MatchError(storage.ErrNotFound))
		})
	})
	When("multiple stores try to create the secret at the same time", func() {
		It("should properly handle conflicts", func() {
			store2, err := storage.NewSecretStoreFromConfig(cfg, testNamespace)
			Expect(err).NotTo(HaveOccurred())

			keyring1 := keyring.New(keyring.NewSharedKeys(randomData()))
			keyring2 := keyring.New(keyring.NewSharedKeys(randomData()))

			wg := errgroup.Group{}
			wg.Go(func() error {
				defer GinkgoRecover()
				for i := 0; i < 50; i++ {
					if err := store.Put(context.Background(), keyring1); err != nil {
						return err
					}
				}
				return nil
			})
			wg.Go(func() error {
				defer GinkgoRecover()
				for i := 0; i < 50; i++ {
					if err := store2.Put(context.Background(), keyring2); err != nil {
						return err
					}
				}
				return nil
			})

			Expect(wg.Wait()).NotTo(HaveOccurred())
		})
	})
	When("creating a new in-cluster store outside of a cluster", func() {
		It("should panic", func() {
			Expect(func() {
				storage.NewInClusterSecretStore()
			}).To(Panic())
			os.Setenv("POD_NAMESPACE", "default")
			Expect(func() {
				storage.NewInClusterSecretStore()
			}).To(Panic())
			os.Unsetenv("POD_NAMESPACE")
		})
	})
	When("the secret contains malformed data", func() {
		BeforeEach(func() {
			// Delete the secret
			err := client.CoreV1().Secrets(testNamespace).Delete(context.Background(),
				"opni-tenant-keyring", metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())
		})
		When("the secret is missing the keyring key", func() {
			It("should error", func() {
				// Create the secret with malformed data
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "opni-tenant-keyring",
						Namespace: testNamespace,
					},
					Data: map[string][]byte{},
				}
				_, err := client.CoreV1().Secrets(testNamespace).
					Create(context.Background(), secret, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				_, err = store.Get(context.Background())
				Expect(err).To(MatchError("secret is missing keyring data"))
			})
		})
		When("the secret does not contain valid json data", func() {
			It("should error", func() {
				// Create the secret with malformed data
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "opni-tenant-keyring",
						Namespace: testNamespace,
					},
					StringData: map[string]string{
						"keyring": `}"invalid": "json"{`,
					},
				}
				_, err := client.CoreV1().Secrets(testNamespace).
					Create(context.Background(), secret, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				_, err = store.Get(context.Background())
				Expect(err).To(BeAssignableToTypeOf(&json.SyntaxError{}))
			})
		})
	})
})
