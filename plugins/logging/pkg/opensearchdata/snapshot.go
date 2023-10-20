package opensearchdata

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/opensearch-project/opensearch-go/opensearchutil"
	opensearchtypes "github.com/rancher/opni/pkg/opensearch/opensearch/types"
	loggingerrors "github.com/rancher/opni/plugins/logging/pkg/errors"
)

func (m *Manager) DoSnapshot(ctx context.Context, repository string, indices []string) error {
	m.WaitForInit()

	snapshotName := fmt.Sprintf("upgrade-%s", time.Now().Format(time.UnixDate))

	settings := opensearchtypes.SnapshotRequest{
		Indices: strings.Join(indices, ","),
	}

	resp, err := m.Client.Snapshot.CreateSnapshot(ctx, snapshotName, repository, opensearchutil.NewJSONReader(settings), false)
	if err != nil {
		return loggingerrors.WrappedOpensearchFailure(err)
	}

	defer resp.Body.Close()

	if resp.IsError() {
		m.logger.Error(fmt.Sprintf("opensearch request failed: %s", resp.String()))
		return loggingerrors.ErrOpensearchResponse
	}

	return nil
}
