package config

import (
	"regexp"
	"strings"

	"dagger.io/dagger"
)

var (
	// generated from docker/distribution
	referenceRegexp = regexp.MustCompile(`^((?:(?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9])(?:(?:\.(?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9]))+)?(?::[0-9]+)?/)?[a-z0-9]+(?:(?:(?:[._]|__|[-]*)[a-z0-9]+)+)?(?:(?:/[a-z0-9]+(?:(?:(?:[._]|__|[-]*)[a-z0-9]+)+)?)+)?)(?::([\w][\w.-]{0,127}))?(?:@([A-Za-z][A-Za-z0-9]*(?:[-_+.][A-Za-z][A-Za-z0-9]*)*[:][[:xdigit:]]{32,}))?$`)
	tagRegexp       = regexp.MustCompile(`[\w][\w.-]{0,127}`)
)

const EnvPrefix = "_OPNI_"

type SpecialCaseEnv struct {
	EnvVar    string
	Keys      []string
	Converter func(k, v string) any
}

func SpecialCaseEnvVars(client *dagger.Client) []SpecialCaseEnv {
	secret := func(k, v string) any {
		return client.SetSecret(k, v)
	}
	plaintext := func(_, v string) any {
		return v
	}

	return []SpecialCaseEnv{
		{
			EnvVar: "DOCKER_USERNAME",
			Keys: []string{
				"images.opni.auth.username",
				"images.minimal.auth.username",
				"images.opensearch.opensearch.auth.username",
				"images.opensearch.dashboards.auth.username",
				"images.opensearch.update-service.auth.username",
				"images.python-base.auth.username",
				"charts.oci.auth.username",
			},
			Converter: plaintext,
		},
		{
			EnvVar: "DOCKER_PASSWORD",
			Keys: []string{
				"images.opni.auth.secret",
				"images.minimal.auth.secret",
				"images.opensearch.opensearch.auth.secret",
				"images.opensearch.dashboards.auth.secret",
				"images.opensearch.update-service.auth.secret",
				"images.python-base.auth.secret",
				"charts.oci.auth.secret",
			},
			Converter: secret,
		},
		{
			EnvVar: "GH_TOKEN",
			Keys: []string{
				"charts.git.auth.secret",
				"releaser.auth.secret",
			},
			Converter: secret,
		},
		{
			EnvVar: "DRONE_BRANCH",
			Keys: []string{
				"images.opni.tag",
				"images.minimal.tag",
				"images.opensearch.opensearch.tag",
				"images.opensearch.dashboards.tag",
				"images.opensearch.update-service.tag",
			},
			Converter: plaintext,
		},
		{
			EnvVar: "DRONE_TAG", // if set, will override DRONE_BRANCH
			Keys: []string{
				"images.opni.tag",
				"images.minimal.tag",
				"images.opensearch.opensearch.tag",
				"images.opensearch.dashboards.tag",
				"images.opensearch.update-service.tag",
				"releaser.tag",
			},
			Converter: plaintext,
		},
	}
}

func SpecialCaseEnvVarHelp() string {
	cases := SpecialCaseEnvVars(nil)
	help := strings.Builder{}
	for _, c := range cases {
		help.WriteString(" " + c.EnvVar + ":\n")
		for _, k := range c.Keys {
			help.WriteString("  - " + k + "\n")
		}
	}
	return help.String()
}
