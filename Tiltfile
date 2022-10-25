load('ext://min_k8s_version', 'min_k8s_version')
load('ext://cert_manager', 'deploy_cert_manager')
load('ext://namespace', 'namespace_create')

set_team('52cc75cc-c4ed-462f-8ea7-a543d398a381')

version = '0.6.0-dev1'
config.define_string_list('allowedContexts')
config.define_string_list('opniChartValues')
config.define_string('defaultRegistry')

cfg = config.parse()

allow_k8s_contexts(cfg.get('allowedContexts'))

min_k8s_version('1.22')
deploy_cert_manager(version='v1.8.0')

namespace_create('opni')

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
  cmd='mage charts',
  ignore=ignore,
)

k8s_yaml(helm('./charts/opni-crd/'+version,
  name='opni-crd',
  namespace='opni',
), allow_duplicates=True)

k8s_yaml(helm('./charts/opni/'+version,
  name='opni',
  namespace='opni',
  set=cfg.get('opniChartValues')
), allow_duplicates=True)

if cfg.get('defaultRegistry') != None:
  default_registry(cfg.get('defaultRegistry'))

custom_build("rancher/opni",
  command="dagger do load opni",
  deps=['controllers', 'apis', 'pkg', 'plugins'], 
  ignore=ignore,
)
