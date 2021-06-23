load('ext://min_k8s_version', 'min_k8s_version')
load('ext://restart_process', 'docker_build_with_restart')

allow_k8s_contexts('k3d-k3s-tilt-opni')
min_k8s_version('1.20')

DIRNAME = os.path.basename(os. getcwd())

k8s_yaml('staging/staging_autogen.yaml')


deps = ['controllers', 'main.go', 'apis', 'pkg/demo', 
    'config/certmanager/kustomization.yaml',
    'config/crd/kustomization.yaml',
    'config/default/kustomization.yaml',
    'config/manager/kustomization.yaml',
    'config/prometheus/kustomization.yaml',
    'config/rbac/kustomization.yaml',
    'config/scorecard/kustomization.yaml',
]
local_resource('Watch & Compile', 
    './scripts/generate && CGO_ENABLED=0 go build -o bin/manager main.go', 
    deps=deps, ignore=['**/zz_generated.deepcopy.go'])

local_resource('Sample YAML', 'kubectl apply -f ./config/samples', 
    deps=["./config/samples"], resource_deps=[DIRNAME + "-controller-manager"])

DOCKERFILE = '''FROM golang:alpine
WORKDIR /
COPY ./bin/manager /
CMD ["/manager"]
'''
docker_build_with_restart("rancher/opni-manager", '.', 
    dockerfile_contents=DOCKERFILE,
    entrypoint='/manager',
    only=['./bin/manager'],
    live_update=[sync('./bin/manager', '/manager')]
)
