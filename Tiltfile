load('ext://min_k8s_version', 'min_k8s_version')
load('ext://cert_manager', 'deploy_cert_manager')
load('ext://helm_remote', 'helm_remote')

settings = read_yaml('tilt-options.yaml', default={})
if "allowedContexts" in settings:
    allow_k8s_contexts(settings["allowedContexts"])

min_k8s_version('1.22')
deploy_cert_manager(version="v1.6.1")

helm_remote('kube-prometheus', 
    repo_name='bitnami', 
    repo_url='https://charts.bitnami.com/bitnami',
    namespace='opni-gateway',
    values="deploy/helm-config/kube-prometheus-values.yaml",
)
helm_remote('etcd', 
    repo_name='bitnami', 
    repo_url='https://charts.bitnami.com/bitnami',
    namespace='opni-gateway',
    values="deploy/helm-config/etcd-values.yaml",
)
helm_remote('cortex',
    repo_name='cortex-helm',
    repo_url='https://cortexproject.github.io/cortex-helm-chart',
    namespace='opni-gateway',
    values="deploy/helm-config/cortex-values.yaml",
)
helm_remote('grafana',
    repo_name='grafana',
    repo_url='https://grafana.github.io/helm-charts',
    namespace='opni-gateway',
    values="deploy/helm-config/grafana-values.yaml",
)

k8s_yaml(kustomize('deploy/gateway'))
k8s_resource(workload='opni-gateway', port_forwards=9090)
k8s_yaml(kustomize('deploy/proxy'))

local_resource('Watch & Compile', 'mage build', 
    deps=['pkg'], ignore=['**/*.pb.go'])

if "defaultRegistry" in settings:
    default_registry(settings["defaultRegistry"])

if "dockerfile" in settings:
    dockerfile = settings["dockerfile"]
else:
    dockerfile = 'Dockerfile'

docker_build("rancher/opni-gateway", '.', dockerfile=dockerfile, 
    ignore=['mage_output_file.go', 'deploy/'])
