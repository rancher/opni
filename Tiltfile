load('ext://min_k8s_version', 'min_k8s_version')
load('ext://cert_manager', 'deploy_cert_manager')
load('ext://helm_remote', 'helm_remote')
load('ext://namespace', 'namespace_create')

settings = read_yaml('tilt-options.yaml', default={})
if "allowedContexts" in settings:
    allow_k8s_contexts(settings["allowedContexts"])

min_k8s_version('1.22')
deploy_cert_manager(version="v1.6.1")

namespace_create('opni-monitoring')

if "hostname" in settings:
    hostname = settings["hostname"]
else:
    fail("hostname is required")

helm_remote('etcd', 
    repo_name='bitnami', 
    repo_url='https://charts.bitnami.com/bitnami',
    namespace='opni-monitoring',
    values="deploy/values/etcd.yaml",
    set=['auth.rbac.rootPassword=tilt']
)
helm_remote('cortex',
    repo_name='cortex-helm',
    repo_url='https://cortexproject.github.io/cortex-helm-chart',
    namespace='opni-monitoring',
    values="deploy/values/cortex.yaml",
)
helm_remote('grafana',
    repo_name='grafana',
    repo_url='https://grafana.github.io/helm-charts',
    namespace='opni-monitoring',
    values=["deploy/values/grafana.yaml", "deploy/custom/grafana.yaml"],
    set=[
        'grafana\\.ini.server.domain=grafana.%s' % hostname,
        'grafana\\.ini.server.root_url=https://grafana.%s' % hostname,
        'tls[0].hosts={grafana.%s}' % hostname,
        'ingress.hosts={grafana.%s}' % hostname,
    ],
)

k8s_yaml(helm('deploy/charts/opni-monitoring',
    name='opni-monitoring',
    namespace='opni-monitoring',
    values=['deploy/custom/opni-monitoring.yaml'],
    set=[
        'gateway.dnsNames={%s}' % hostname,
        'management.grpcListenAddress=tcp://0.0.0.0:11090',
        'management.httpListenAddress=0.0.0.0:11080',
        'management.webListenAddress=0.0.0.0:12080',
    ]
))

k8s_resource(workload='opni-gateway', port_forwards=[11090, 11080, 12080])

local_resource('Watch & Compile', 'mage build', 
    deps=['pkg'], ignore=[
        '**/*.pb.go',
        '**/*.pb.*.go',
        '**/*.swagger.json',
        'pkg/test/mock/*',
    ])

if "defaultRegistry" in settings:
    default_registry(settings["defaultRegistry"])

if "dockerfile" in settings:
    dockerfile = settings["dockerfile"]
else:
    dockerfile = 'Dockerfile'

docker_build("kralicky/opni-monitoring", '.', dockerfile=dockerfile, 
    ignore=['mage_output_file.go', 'deploy/'])
