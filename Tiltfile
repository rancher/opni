load('ext://min_k8s_version', 'min_k8s_version')
load('ext://restart_process', 'docker_build_with_restart')

min_k8s_version('1.18')
allow_k8s_contexts('k3d-k3s-tilt-opni')

# Uncomment the following line if you are not using the k3d cluster created
# via ./hack/create-k3d-cluster.sh, and change the context name accordingly.
#
# allow_k8s_contexts('your-context-name')

DIRNAME = os.path.basename(os. getcwd())

k8s_yaml('staging/staging_autogen.yaml')

deps = ['controllers', 'main.go', 'apis', 'pkg/demo', 'pkg/util/manager',
    'config/certmanager/kustomization.yaml',
    'config/crd/kustomization.yaml',
    'config/default/kustomization.yaml',
    'config/manager/kustomization.yaml',
    'config/prometheus/kustomization.yaml',
    'config/rbac/kustomization.yaml',
    'config/scorecard/kustomization.yaml',
]
local_resource('Watch & Compile', 
    './scripts/generate && CGO_ENABLED=0 go build -gcflags="all=-N -l" -o bin/manager main.go', 
    deps=deps, 
    ignore=['**/zz_generated.deepcopy.go'],
)

local_resource('Sample YAML', 'kubectl apply -k ./config/samples', 
    deps=["./config/samples"], resource_deps=[DIRNAME + "-controller-manager"])

DOCKERFILE = '''FROM golang:alpine
RUN apk add --no-cache git
RUN go get github.com/go-delve/delve/cmd/dlv
EXPOSE 40000
WORKDIR /workspace
COPY . /workspace
ENTRYPOINT ["/go/bin/dlv"]
'''

# Uncomment the following line if you are not using the k3d cluster created
# via ./hack/create-k3d-cluster.sh, and ensure a docker hub repo exists 
# at yourusername/opni-manager.
#
# default_registry('docker.io/yourusername')

docker_build_with_restart("rancher/opni-manager", '.', 
    dockerfile_contents=DOCKERFILE,
    live_update=[sync('./bin/manager', '/workspace/bin/manager')],
    entrypoint=[
        '/go/bin/dlv', "--listen=:40000", "--api-version=2", "--headless=true", 
        "exec", "--continue", "--accept-multiclient", "/workspace/bin/manager", 
        "--",
    ],
)

k8s_resource(workload='opni-controller-manager', port_forwards=['40000:40000'])
