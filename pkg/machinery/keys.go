package machinery

import (
	"path/filepath"

	"github.com/spf13/afero"
	"github.com/ttacon/chalk"
	"go.uber.org/zap"

	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/keyring/ephemeral"
	"github.com/rancher/opni/pkg/logger"
)

func LoadEphemeralKeys(fsys afero.Afero, dirs ...string) ([]*keyring.EphemeralKey, error) {
	keyringLog := logger.New().Named("keyring")
	var keys []*keyring.EphemeralKey

	for _, dir := range dirs {
		infos, err := fsys.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		for _, info := range infos {
			if info.IsDir() {
				continue
			}
			perm := info.Mode().Perm()
			path := filepath.Join(dir, info.Name())
			lg := keyringLog.With("path", path)
			if perm&0040 > 0 {
				lg.Warn(chalk.Yellow.Color("Ephemeral key is group-readable. This is insecure."))
			}
			if perm&0004 > 0 {
				lg.Warn(chalk.Yellow.Color("Ephemeral key is world-readable. This is insecure."))
			}

			f, err := fsys.Open(path)
			if err != nil {
				return nil, err
			}
			ekey, err := ephemeral.LoadKey(f)
			f.Close()
			if err != nil {
				lg.With(
					zap.Error(err),
				).Error("failed to load ephemeral key, skipping")
				continue
			}
			lg.With(
				"usage", ekey.Usage,
				"labels", ekey.Labels,
			).Debug("loaded ephemeral key")

			keys = append(keys, ekey)
		}
	}

	return keys, nil
}
