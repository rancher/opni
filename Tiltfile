load('ext://restart_process', 'docker_build_with_restart')

allow_k8s_contexts('k3d-k3s-tilt-opni')
allow_k8s_contexts('k3s')

k8s_yaml('staging/staging_autogen.yaml')

deps = ['controllers', 'main.go', 'apis', 'pkg/demo', 'pkg/util/manager',
        'pkg/resources', 'pkg/providers']

local_resource('Watch & Compile', 
    './scripts/generate && CGO_ENABLED=0 GOOS=linux go build -o bin/manager main.go', 
    deps=deps, ignore=['**/zz_generated.deepcopy.go'])

local_resource('Sample YAML', 'kubectl apply -k ./config/samples', 
    deps=["./config/samples"], resource_deps=["opni-controller-manager"])

DOCKERFILE = '''FROM golang:alpine
WORKDIR /
COPY ./bin/manager /
CMD ["/manager"]
'''
# default_registry(
#     'docker.io/joekralicky',
# )
docker_build_with_restart("rancher/opni-manager", '.', 
    dockerfile_contents=DOCKERFILE,
    entrypoint=['/manager'],
    only=['./bin/manager'],
    live_update=[sync('./bin/manager', '/manager')]
)
