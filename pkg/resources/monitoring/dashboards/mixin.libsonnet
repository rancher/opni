local kubernetes = import 'kubernetes-mixin/mixin.libsonnet';

kubernetes {
  _config+:: {
    cadvisorSelector: 'job="kubelet"',
    showMultiCluster: true,
    clusterLabel: '__tenant_id__',
    datasourceName: 'Opni',
  },
}
