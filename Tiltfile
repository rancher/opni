load('ext://min_k8s_version', 'min_k8s_version')
load('ext://helm_resource', 'helm_resource')
load('ext://namespace', 'namespace_create')

set_team('52cc75cc-c4ed-462f-8ea7-a543d398a381')

version = '0.10.0'
config.define_string_list('allowedContexts')
config.define_string_list('opniChartValues')
config.define_string('defaultRegistry')
config.define_string('valuesPath')

cfg = config.parse()

allow_k8s_contexts(cfg.get('allowedContexts'))

min_k8s_version('1.24')

namespace_create('opni')

update_settings (
  max_parallel_updates=1,
  k8s_upsert_timeout_secs=300,
)

ignore=[
  '**/*.pb.go',
  '**/*.pb.*.go',
  '**/*.swagger.json',
  'pkg/test/mock/*',
  'pkg/sdk/crd/*',
  '**/zz_generated.*',
  'packages/'
]

if cfg.get('defaultRegistry') != None:
  default_registry(cfg.get('defaultRegistry'))

local_resource('build charts',
  deps='packages/**/templates',
  cmd='go run ./dagger --charts.git.export',
  ignore=ignore,
)

helm_resource('opni-crd', './charts/opni-crd/'+version,
  namespace='opni',
  release_name='opni-crd',
  resource_deps=['build charts'],
  auto_init=False
)
# workaround for https://github.com/tilt-dev/tilt/issues/6058
k8s_resource('opni-crd', auto_init=True, pod_readiness='ignore')

helm_resource('opni', './charts/opni/'+version,
  namespace='opni',
  release_name='opni',
  resource_deps=['build charts', 'opni-crd'],
  image_selector="rancher/opni",
  image_deps=['rancher/opni'],
  image_keys=[("image.repository", "image.tag")],
)

custom_build("rancher/opni",
  command="go run ./dagger --images.opni.push --images.opni.repo=${EXPECTED_REF/:*} --images.opni.tag=${EXPECTED_REF/*:}",
  deps=['controllers', 'apis', 'pkg', 'plugins'],
  ignore=ignore,
  skips_local_docker=True,
)
