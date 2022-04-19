load('ext://min_k8s_version', 'min_k8s_version')
load('ext://cert_manager', 'deploy_cert_manager')

set_team('52cc75cc-c4ed-462f-8ea7-a543d398a381')

settings = read_yaml('tilt-options.yaml', default={})

if "allowedContexts" in settings:
  allow_k8s_contexts(settings["allowedContexts"])

min_k8s_version('1.22')
deploy_cert_manager(version="v1.8.0")
k8s_yaml(kustomize('config/default'))

deps = ['controllers', 'apis', 'pkg']

local_resource('Watch & Compile', 
  'mage', 
  deps=deps, ignore=[
    '**/*.pb.go',
    '**/*.pb.*.go',
    '**/*.swagger.json',
    'pkg/test/mock/*',
    'pkg/sdk/crd/*',
    '**/zz_generated.*',
  ])

local_resource('Sample YAML', 'kubectl apply -k ./config/samples', 
  deps=["./config/samples"], resource_deps=["opni-controller-manager"], trigger_mode=TRIGGER_MODE_MANUAL, auto_init=False)

local_resource('Deployment YAML', 'kubectl apply -k ./deploy', 
  deps=["./config/deploy"], resource_deps=["opni-controller-manager"], trigger_mode=TRIGGER_MODE_MANUAL, auto_init=False)

DOCKERFILE = '''FROM golang:alpine
WORKDIR /
COPY ./bin/opni /usr/bin/opni
COPY ./package/assets/nfd/ /opt/nfd/
COPY ./package/assets/gpu-operator/ /opt/gpu-operator/
ENTRYPOINT ["/usr/bin/opni"]
'''

if "defaultRegistry" in settings:
  default_registry(settings["defaultRegistry"])

docker_build("rancher/opni", '.', 
  dockerfile_contents=DOCKERFILE,
  only=['./bin/opni', './package/assets'],
  # live_update=[sync('./bin/opni', '/opni')]
)

include('Tiltfile.tests')
