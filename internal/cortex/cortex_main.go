package cortex_internal

import (
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"github.com/weaveworks/common/tracing"
	"gopkg.in/yaml.v2"

	"github.com/cortexproject/cortex/pkg/cortex"
	"github.com/cortexproject/cortex/pkg/util/flagext"
	util_log "github.com/cortexproject/cortex/pkg/util/log"
)

////////////////////////////////////////////////////////////////////////////////
// The initialization code below is copied from cortex/cmd/cortex/main.go.    //
// Some unused features have been removed (e.g. test mode).                   //
////////////////////////////////////////////////////////////////////////////////

const (
	configFileOption = "config.file"
	configExpandENV  = "config.expand-env"
)

func Main(args []string) {
	var cfg cortex.Config
	var printModules bool
	configFile, expandENV := parseConfigFileParameter(args[1:])

	// This sets default values from flags to the config.
	// It needs to be called before parsing the config file!
	flagext.RegisterFlags(&cfg)

	if configFile != "" {
		if err := LoadConfig(configFile, expandENV, &cfg); err != nil {
			fmt.Fprintf(os.Stderr, "error loading config from %s: %v\n", configFile, err)
			os.Exit(1)
		}
	}

	// Ignore -config.file and -config.expand-env here, since it was already parsed, but it's still present on command line.
	flagext.IgnoredFlag(flag.CommandLine, configFileOption, "Configuration file to load.")
	_ = flag.CommandLine.Bool(configExpandENV, false, "Expands ${var} or $var in config according to the values of the environment variables.")
	flag.BoolVar(&printModules, "modules", false, "List available values that can be used as target.")

	usage := flag.CommandLine.Usage
	flag.CommandLine.Usage = func() { /* don't do anything by default, we will print usage ourselves, but only when requested. */ }
	flag.CommandLine.Init(flag.CommandLine.Name(), flag.ContinueOnError)

	err := flag.CommandLine.Parse(args[1:])
	if err == flag.ErrHelp {
		// Print available parameters to stdout, so that users can grep/less it easily.
		flag.CommandLine.SetOutput(os.Stdout)
		usage()
		os.Exit(2)
	} else if err != nil {
		fmt.Fprintln(flag.CommandLine.Output(), "Run with -help to get list of available parameters")
		os.Exit(2)
	}

	// Validate the config once both the config file has been loaded
	// and CLI flags parsed.
	err = cfg.Validate(util_log.Logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error validating config: %v\n", err)
		os.Exit(1)
	}

	util_log.InitLogger(&cfg.Server)

	name := "cortex"
	if len(cfg.Target) == 1 {
		name += "-" + cfg.Target[0]
	}

	//////////////////////////////////////////////////////////////////////////////
	// Opni Custom Logic: Translate jaeger config
	if os.Getenv("OTEL_TRACES_EXPORTER") == "jaeger" {
		endpoint := os.Getenv("OTEL_EXPORTER_JAEGER_ENDPOINT")
		if endpoint == "" {
			endpoint = "http://localhost:14268/api/traces"
		}
		os.Setenv("JAEGER_AGENT_HOST", endpoint)
	}
	//////////////////////////////////////////////////////////////////////////////

	// Setting the environment variable JAEGER_AGENT_HOST enables tracing.
	if trace, err := tracing.NewFromEnv(name); err != nil {
		level.Error(util_log.Logger).Log("msg", "Failed to setup tracing", "err", err.Error())
	} else {
		defer trace.Close()
	}

	// Initialise seed for randomness usage.
	rand.Seed(time.Now().UnixNano())

	t, err := cortex.New(cfg)

	//////////////////////////////////////////////////////////////////////////////
	// Opni Custom Logic: Run the compactor in single-binary mode
	t.ModuleManager.AddDependency(cortex.All, cortex.Compactor)
	//////////////////////////////////////////////////////////////////////////////

	util_log.CheckFatal("initializing cortex", err)
	if printModules {
		allDeps := t.ModuleManager.DependenciesForModule(cortex.All)

		for _, m := range t.ModuleManager.UserVisibleModuleNames() {
			ix := sort.SearchStrings(allDeps, m)
			included := ix < len(allDeps) && allDeps[ix] == m

			if included {
				fmt.Fprintln(os.Stdout, m, "*")
			} else {
				fmt.Fprintln(os.Stdout, m)
			}
		}

		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, "Modules marked with * are included in target All.")
		return
	}
	level.Info(util_log.Logger).Log("msg", "Starting Cortex", "version", "(opni embedded)")
	err = t.Run()

	util_log.CheckFatal("running cortex", err)
}

// Parse -config.file and -config.expand-env option via separate flag set, to avoid polluting default one and calling flag.Parse on it twice.
func parseConfigFileParameter(args []string) (configFile string, expandEnv bool) {
	// ignore errors and any output here. Any flag errors will be reported by main flag.Parse() call.
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.SetOutput(ioutil.Discard)

	// usage not used in these functions.
	fs.StringVar(&configFile, configFileOption, "", "")
	fs.BoolVar(&expandEnv, configExpandENV, false, "")

	// Try to find -config.file and -config.expand-env option in the flags. As Parsing stops on the first error, eg. unknown flag, we simply
	// try remaining parameters until we find config flag, or there are no params left.
	// (ContinueOnError just means that flag.Parse doesn't call panic or os.Exit, but it returns error, which we ignore)
	for len(args) > 0 {
		_ = fs.Parse(args)
		args = args[1:]
	}

	return
}

// LoadConfig read YAML-formatted config from filename into cfg.
func LoadConfig(filename string, expandENV bool, cfg *cortex.Config) error {
	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		return errors.Wrap(err, "Error reading config file")
	}

	if expandENV {
		buf = expandEnv(buf)
	}

	err = yaml.UnmarshalStrict(buf, cfg)
	if err != nil {
		return errors.Wrap(err, "Error parsing config file")
	}

	return nil
}

// expandEnv replaces ${var} or $var in config according to the values of the current environment variables.
// The replacement is case-sensitive. References to undefined variables are replaced by the empty string.
// A default value can be given by using the form ${var:default value}.
func expandEnv(config []byte) []byte {
	return []byte(os.Expand(string(config), func(key string) string {
		keyAndDefault := strings.SplitN(key, ":", 2)
		key = keyAndDefault[0]

		v := os.Getenv(key)
		if v == "" && len(keyAndDefault) == 2 {
			v = keyAndDefault[1] // Set value to the default.
		}
		return v
	}))
}
