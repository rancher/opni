load('ext://min_k8s_version', 'min_k8s_version')
load('ext://helm_resource', 'helm_resource')
load('ext://namespace', 'namespace_create')

set_team('52cc75cc-c4ed-462f-8ea7-a543d398a381')

version = '0.11.0-rc2'
config.define_string_list('allowedContexts')
config.define_string_list('opniChartValues')
config.define_string('defaultRegistry')
config.define_string('valuesPath')

cfg = config.parse()

allow_k8s_contexts(cfg.get('allowedContexts'))

min_k8s_version('1.23')

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

local_resource('build charts',
  deps='packages/**/templates',
  cmd='go run ./dagger --charts.git.export',
  ignore=ignore,
)

k8s_yaml(helm('./charts/opni-crd/'+version,
  name='opni-crd',
  namespace='opni',
), allow_duplicates=True)

if cfg.get('valuesPath') != None:
  k8s_yaml(helm('./charts/opni/'+version,
    name='opni',
    namespace='opni',
    values=cfg.get('valuesPath')
  ), allow_duplicates=True)
else:
  k8s_yaml(helm('./charts/opni/'+version,
    name='opni',
    namespace='opni',
    set=cfg.get('opniChartValues')
  ), allow_duplicates=True)

if cfg.get('defaultRegistry') != None:
  default_registry(cfg.get('defaultRegistry'))

custom_build("rancher/opni",
  command="go run ./dagger --images.opni.push --images.opni.repo=${EXPECTED_REF/:*} --images.opni.tag=${EXPECTED_REF/*:}",
  deps=['controllers', 'apis', 'pkg', 'plugins'],
  ignore=ignore,
  skips_local_docker=True,
)
