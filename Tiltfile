load('ext://min_k8s_version', 'min_k8s_version')
load('ext://cert_manager', 'deploy_cert_manager')

set_team('52cc75cc-c4ed-462f-8ea7-a543d398a381')

settings = read_yaml('tilt-options.yaml', default={})

if "allowedContexts" in settings:
    allow_k8s_contexts(settings["allowedContexts"])

# min_k8s_version('1.22')
deploy_cert_manager(version="v1.5.3")
k8s_yaml(kustomize('config/default'))

deps = ['controllers', 'main.go', 'apis', 'pkg/demo', 'pkg/util/manager',
        'pkg/resources', 'pkg/providers']

local_resource('Watch & Compile', 
    './scripts/generate && CGO_ENABLED=0 GOOS=linux go build -o bin/manager main.go', 
    deps=deps, ignore=['**/zz_generated.deepcopy.go'])

local_resource('Sample YAML', 'kubectl apply -k ./config/samples', 
    deps=["./config/samples"], resource_deps=["opni-controller-manager"], trigger_mode=TRIGGER_MODE_MANUAL, auto_init=False)

local_resource('Deployment YAML', 'kubectl apply -k ./deploy', 
    deps=["./config/deploy"], resource_deps=["opni-controller-manager"], trigger_mode=TRIGGER_MODE_MANUAL, auto_init=False)

DOCKERFILE = '''FROM golang:alpine
WORKDIR /
COPY ./bin/manager /
COPY ./package/assets/nfd/ /opt/nfd/
COPY ./package/assets/gpu-operator/ /opt/gpu-operator/
ENTRYPOINT ["/manager"]
'''

if "defaultRegistry" in settings:
    default_registry(settings["defaultRegistry"])

docker_build("rancher/opni-manager", '.', 
    dockerfile_contents=DOCKERFILE,
    only=['./bin/manager', './package/assets'],
)

include('Tiltfile.tests')
