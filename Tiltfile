load('ext://min_k8s_version', 'min_k8s_version')
load('ext://cert_manager', 'deploy_cert_manager')

set_team('52cc75cc-c4ed-462f-8ea7-a543d398a381')

settings = read_yaml('tilt-options.yaml', default={})

if "allowedContexts" in settings:
  allow_k8s_contexts(settings["allowedContexts"])

min_k8s_version('1.22')
deploy_cert_manager(version="v1.8.0")

bundle = local("curl -fsSL https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/v0.56.0/bundle.yaml", quiet=True)
k8s_yaml(bundle)


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
  deps=["./config/samples"], resource_deps=["opni-manager"], trigger_mode=TRIGGER_MODE_MANUAL, auto_init=False)

local_resource('Deployment YAML', 'kubectl apply -k ./deploy', 
  deps=["./config/deploy"], resource_deps=["opni-manager"], trigger_mode=TRIGGER_MODE_MANUAL, auto_init=False)

if "hostname" in settings:
  objects = read_yaml_stream('config/samples/tilt/gateway.yaml')
  ns = objects[0]
  gateway = objects[1]
  gateway["spec"]["hostname"] = settings["hostname"]
  k8s_yaml(encode_yaml_stream([ns, gateway]))

  monitoring = read_yaml('config/samples/tilt/monitoring.yaml')
  monitoring["spec"]["grafana"]["hostname"] = "grafana."+settings["hostname"]
  k8s_yaml(encode_yaml(monitoring))

DOCKERFILE = '''FROM golang:alpine
WORKDIR /
RUN apk add --no-cache curl
COPY ./bin/opni /usr/bin/opni
COPY ./bin/plugins/plugin_cortex /var/lib/opni/plugins/
COPY ./bin/plugins/plugin_logging /var/lib/opni/plugins/
COPY ./config/assets/nfd/ /opt/nfd/
COPY ./config/assets/gpu-operator/ /opt/gpu-operator/
ENTRYPOINT ["/usr/bin/opni"]
'''

if "defaultRegistry" in settings:
  default_registry(settings["defaultRegistry"])

docker_build("rancher/opni", '.', 
  dockerfile_contents=DOCKERFILE,
  only=['./bin', './config/assets'],
  # live_update=[sync('./bin/opni', '/opni')]
)

# include('Tiltfile.tests')
