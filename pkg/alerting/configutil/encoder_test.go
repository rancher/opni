package configutil_test

import (
	"bytes"
	"net/url"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	amcfg "github.com/prometheus/alertmanager/config"
	commoncfg "github.com/prometheus/common/config"
	"github.com/rancher/opni/pkg/alerting/configutil"
	"github.com/samber/lo"
	"gopkg.in/yaml.v3"
)

type fictionalMegaSecret struct {
	// amcfg.Secrets

	AMSecret         amcfg.Secret
	AMSecretPtr      *amcfg.Secret
	AMSecretSlice    []amcfg.Secret
	AMSecretSlicePtr []*amcfg.Secret
	AMSecretMap      map[string]amcfg.Secret
	AMSecretMapPtr   map[string]*amcfg.Secret

	// amcfg.SecretURLs

	AMSecretURL         amcfg.SecretURL
	AMSecretURLPtr      *amcfg.SecretURL
	AMSecretURLSlice    []amcfg.SecretURL
	AMSecretURLSlicePtr []*amcfg.SecretURL
	AMSecretURLMap      map[string]amcfg.SecretURL
	AMSecretURLMapPtr   map[string]*amcfg.SecretURL

	// commoncfg.Secrets

	CMSecret         commoncfg.Secret
	CMSecretPtr      *commoncfg.Secret
	CMSecretSlice    []commoncfg.Secret
	CMSecretSlicePtr []*commoncfg.Secret
	CMSecretMap      map[string]commoncfg.Secret
	CMSecretMapPtr   map[string]*commoncfg.Secret
}

const (
	redacted = "<secret>"
)

var (
	megaSecret = fictionalMegaSecret{
		AMSecret:    amcfg.Secret("secret"),
		AMSecretPtr: lo.ToPtr(amcfg.Secret("secretPtr")),
		AMSecretSlice: []amcfg.Secret{
			"secretSlice1",
			"secretSlice2",
			"secretSlice3",
		},
		AMSecretSlicePtr: []*amcfg.Secret{
			lo.ToPtr(amcfg.Secret("secretSlicePtr1")),
			lo.ToPtr(amcfg.Secret("secretSlicePtr2")),
			lo.ToPtr(amcfg.Secret("secretSlicePtr3")),
		},
		AMSecretMap: map[string]amcfg.Secret{
			"key1": "secretMap1",
			"key2": "secretMap2",
		},
		AMSecretMapPtr: map[string]*amcfg.Secret{
			"key1": lo.ToPtr(amcfg.Secret("secretMap1")),
			"key2": lo.ToPtr(amcfg.Secret("secretMap1")),
		},
		AMSecretURL: amcfg.SecretURL{URL: &url.URL{
			Scheme: "http",
			Host:   "host",
		}},
		AMSecretURLPtr: &amcfg.SecretURL{URL: &url.URL{
			Scheme: "http",
			Host:   "host",
		}},
		AMSecretURLSlice: []amcfg.SecretURL{
			{URL: &url.URL{
				Scheme: "http",
				Host:   "host",
			},
			}},
		AMSecretURLSlicePtr: []*amcfg.SecretURL{
			{URL: &url.URL{
				Scheme: "http",
				Host:   "host",
			}},
		},
		AMSecretURLMap: map[string]amcfg.SecretURL{
			"key1": {URL: &url.URL{
				Scheme: "http",
				Host:   "host",
			}},
		},
		AMSecretURLMapPtr: map[string]*amcfg.SecretURL{
			"key1": {URL: &url.URL{
				Scheme: "http",
				Host:   "host",
			}},
		},
		CMSecret:    commoncfg.Secret("secret"),
		CMSecretPtr: lo.ToPtr(commoncfg.Secret("secretPtr")),
		CMSecretSlice: []commoncfg.Secret{
			"cm_secret",
		},
		CMSecretSlicePtr: []*commoncfg.Secret{
			lo.ToPtr(commoncfg.Secret("cm_secret_ptr")),
		},
		CMSecretMap: map[string]commoncfg.Secret{
			"key1": "cm_secret_map",
		},
		CMSecretMapPtr: map[string]*commoncfg.Secret{
			"key1": lo.ToPtr(commoncfg.Secret("cm_secret_map_ptr")),
		},
	}
)

var _ = Describe("Encoder test", func() {
	When("We use the secret encoder overrider", func() {
		It("should not redact secrets when using the override encoder", func() {
			buf := new(bytes.Buffer)
			encoder := configutil.NewSecretOverrideEncoder(buf)
			err := encoder.Encode(megaSecret)
			Expect(err).ToNot(HaveOccurred())

			var decodedSecret fictionalMegaSecret
			err = yaml.Unmarshal(buf.Bytes(), &decodedSecret)
			Expect(err).To(Succeed())
			Expect(megaSecret).To(Equal(decodedSecret))
		})

		It("should redact secrets when using the default v3 encoder", func() {
			buf := new(bytes.Buffer)
			encoder := yaml.NewEncoder(buf)
			err := encoder.Encode(megaSecret)
			Expect(err).ToNot(HaveOccurred())

			var secret fictionalMegaSecret
			err = yaml.Unmarshal(buf.Bytes(), &secret)
			Expect(err).To(Succeed())

			Expect(secret.AMSecret).To(Equal(amcfg.Secret(redacted)))
			Expect(*secret.AMSecretPtr).To(Equal(amcfg.Secret(redacted)))

			for _, s := range secret.AMSecretSlice {
				Expect(s).To(Equal(amcfg.Secret(redacted)))
			}

			for _, s := range secret.AMSecretSlicePtr {
				Expect(*s).To(Equal(amcfg.Secret(redacted)))
			}

			for _, s := range secret.AMSecretMap {
				Expect(s).To(Equal(amcfg.Secret(redacted)))
			}

			for _, s := range secret.AMSecretMapPtr {
				Expect(*s).To(Equal(amcfg.Secret(redacted)))
			}

			Expect(secret.CMSecret).To(Equal(commoncfg.Secret(redacted)))
			Expect(*secret.CMSecretPtr).To(Equal(commoncfg.Secret(redacted)))

			for _, s := range secret.CMSecretSlice {
				Expect(s).To(Equal(commoncfg.Secret(redacted)))
			}

			for _, s := range secret.CMSecretSlicePtr {
				Expect(*s).To(Equal(commoncfg.Secret(redacted)))
			}

			for _, s := range secret.CMSecretMap {
				Expect(s).To(Equal(commoncfg.Secret(redacted)))
			}

			for _, s := range secret.CMSecretMapPtr {
				Expect(*s).To(Equal(commoncfg.Secret(redacted)))
			}

			Expect(secret.AMSecretURL).To(Equal(amcfg.SecretURL{
				URL: &url.URL{},
			}))
			Expect(*secret.AMSecretURLPtr).To(Equal(amcfg.SecretURL{
				URL: &url.URL{},
			}))

			for _, s := range secret.AMSecretURLSlice {
				Expect(s).To(Equal(amcfg.SecretURL{
					URL: &url.URL{},
				}))
			}

			for _, s := range secret.AMSecretURLSlicePtr {
				Expect(*s).To(Equal(amcfg.SecretURL{
					URL: &url.URL{},
				}))
			}

			for _, s := range secret.AMSecretURLMap {
				Expect(s).To(Equal(amcfg.SecretURL{
					URL: &url.URL{},
				}))
			}

			for _, s := range secret.AMSecretURLMapPtr {
				Expect(*s).To(Equal(amcfg.SecretURL{
					URL: &url.URL{},
				}))
			}
		})
	})
})
