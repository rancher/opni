load('ext://min_k8s_version', 'min_k8s_version')

settings = read_yaml('tilt-options.yaml', default={})

if "allowedContexts" in settings:
    allow_k8s_contexts(settings["allowedContexts"])

# min_k8s_version('1.22')

k8s_yaml('staging/staging_autogen.yaml')

deps = ['controllers', 'main.go', 'apis', 'pkg/demo', 'pkg/util/manager',
        'pkg/resources', 'pkg/providers']

local_resource('Watch & Compile', 
    './scripts/generate && CGO_ENABLED=0 GOOS=linux go build -o bin/manager main.go', 
    deps=deps, ignore=['**/zz_generated.deepcopy.go'])

local_resource('Sample YAML', 'kubectl apply -k ./config/samples', 
    deps=["./config/samples"], resource_deps=["opni-controller-manager"],
    auto_init=False, trigger_mode=TRIGGER_MODE_MANUAL)

DOCKERFILE = '''FROM golang:alpine
WORKDIR /
COPY ./bin/manager /
COPY ./config/assets/nfd/ /opt/nfd/
COPY ./config/assets/gpu-operator/ /opt/gpu-operator/
ENTRYPOINT ["/manager"]
'''

if "defaultRegistry" in settings:
    default_registry(settings["defaultRegistry"])

docker_build("rancher/opni-manager", '.', 
    dockerfile_contents=DOCKERFILE,
    container_args=['--feature-gates=AllAlpha=true'],
    only=['./bin/manager', './config/assets'],
    live_update=[sync('./bin/manager', '/manager')]
)
