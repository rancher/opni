package targets

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"reflect"
	"strings"

	"github.com/bmatcuk/doublestar"
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

type Test mg.Namespace

// Runs all tests
func (Test) All() error {
	if testbinNeedsUpdate() {
		mg.Deps(Test.Bin)
	}
	return sh.RunWithV(map[string]string{
		"CGO_ENABLED": "1",
	}, mg.GoCmd(), "test", "-race", "./...")
}

// Runs all tests with coverage analysis
func (Test) Cover() error {
	if testbinNeedsUpdate() {
		mg.Deps(Test.Bin)
	}
	err := sh.RunWithV(map[string]string{
		"CGO_ENABLED": "1",
	}, mg.GoCmd(), "test",
		"-race",
		"-cover",
		"-coverprofile=cover.out",
		"-coverpkg=./...",
		"./...",
	)
	if err != nil {
		return err
	}
	fmt.Print("processing coverage report... ")
	defer fmt.Println("done.")
	return filterCoverage("cover.out", []string{
		"**/*.pb.go",
		"**/*.pb*.go",
		"**/zz_*.go",
	})
}

func filterCoverage(report string, patterns []string) error {
	f, err := os.Open(report)
	if err != nil {
		return err
	}
	defer f.Close()

	tempFile := fmt.Sprintf(".%s.tmp", report)
	tf, err := os.Create(tempFile)
	if err != nil {
		return err
	}

	patternIndex := 0
	scan := bufio.NewScanner(f)
	scan.Scan() // mode line
	tf.WriteString(scan.Text() + "\n")
LINES:
	for scan.Scan() {
		line := scan.Text()
		filename, _, _ := strings.Cut(line, ":")
		var j int
		for i := patternIndex; j < len(patterns); i = (i + 1) % len(patterns) {
			match, _ := doublestar.Match(patterns[i], filename)
			if match {
				continue LINES
			}
			j++
		}
		tf.WriteString(line + "\n")
	}
	if err := scan.Err(); err != nil {
		return err
	}
	tf.Close()

	return os.Rename(tempFile, report)
}

// Runs the test environment
func (Test) Env() {
	// check if testbin exists
	deps := []any{Build.Testenv}
	if testbinNeedsUpdate() {
		mg.Deps(Test.Bin)
	}
	mg.Deps(deps...)
	cmd := exec.Command("bin/testenv", "--agent-id-seed=0")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, os.Interrupt)
	if err := cmd.Start(); err != nil {
		panic(err)
	}
	proc := cmd.Process
	go func() {
		<-sigint
		proc.Signal(os.Interrupt)
	}()
	if err := cmd.Wait(); err != nil {
		panic(err)
	}
}

const k8sVersion = "1.26.3"

var testbinConfig = fmt.Sprintf(`
{
	"binaries": [
		{
			"name": "etcd",
			"sourceImage": "bitnami/etcd",
			"path": "/opt/bitnami/etcd/bin/etcd"
		},
		{
			"name": "prometheus",
			"sourceImage": "prom/prometheus",
			"path": "/bin/prometheus"
		},
		{
			"name": "promtool",
			"sourceImage": "prom/prometheus",
			"path": "/bin/promtool"
		},
		{
			"name": "node_exporter",
			"sourceImage": "prom/node-exporter",
			"path": "/bin/node_exporter"
		},
		{
			"name": "alertmanager",
			"sourceImage": "prom/alertmanager",
			"path": "/bin/alertmanager"
		},
		{
			"name": "amtool",
			"sourceImage": "prom/alertmanager",
			"path": "/bin/amtool"
		},
		{
			"name": "nats-server",
			"sourceImage": "nats",
			"path": "/nats-server"
		},
		{
			"name": "otelcol-custom",
			"sourceImage": "ghcr.io/rancher-sandbox/opni-otel-collector",
			"version": "v0.1.2-0.74.0",
			"path": "/otelcol-custom"
		},
		{
			"name": "kube-apiserver",
			"sourceImage": "registry.k8s.io/kube-apiserver",
			"version": "v%[1]s",
			"path": "/usr/local/bin/kube-apiserver"
		},
		{
			"name": "kube-controller-manager",
			"sourceImage": "registry.k8s.io/kube-controller-manager",
			"version": "v%[1]s",
			"path": "/usr/local/bin/kube-controller-manager"
		},
		{
			"name": "kubectl",
			"sourceImage": "bitnami/kubectl",
			"version": "%[1]s",
			"path": "/opt/bitnami/kubectl/bin/kubectl"
		}
	]
}`[1:], k8sVersion)

// Prints the testbin configuration to stdout
func (Test) BinConfig() {
	fmt.Println(testbinConfig)
}

func testbinNeedsUpdate() bool {
	lock, err := os.ReadFile("testbin/lock.json")
	if err != nil {
		return true
	}
	var current, desired map[string]any
	if err := json.Unmarshal(lock, &current); err != nil {
		return true
	}
	if err := json.Unmarshal([]byte(testbinConfig), &desired); err != nil {
		return true
	}
	return reflect.DeepEqual(current, desired)
}

// Creates or rebuilds the testbin directory
func (Test) Bin() error {
	if _, err := os.Stat("testbin"); err == nil {
		os.RemoveAll("testbin")
	}
	return Dagger{}.do(".", "testbin", "--config", testbinConfig)
}
