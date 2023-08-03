package ext

import (
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
)

func (c *SampleConfiguration) WithRevision(rev int64) *SampleConfiguration {
	c.Revision = corev1.NewRevision(rev)
	return c
}

func (c *SampleConfiguration) WithoutRevision() *SampleConfiguration {
	c.Revision = nil
	return c
}
