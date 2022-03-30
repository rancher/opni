load('ext://min_k8s_version', 'min_k8s_version')
load('ext://cert_manager', 'deploy_cert_manager')
load('ext://helm_remote', 'helm_remote')
load('ext://helm_resource', 'helm_resource')
load('ext://helm_resource', 'helm_repo')
load('ext://namespace', 'namespace_create')

settings = read_yaml('tilt-options.yaml', default={})
if "allowedContexts" in settings:
    allow_k8s_contexts(settings["allowedContexts"])

agent_mode = False
if "agentContexts" in settings:
    if k8s_context() in settings["agentContexts"]:
        allow_k8s_contexts(k8s_context())
        agent_mode = True

min_k8s_version('1.22')

if agent_mode:
    namespace_create('opni-monitoring-agent')
    namespace_create('monitoring')
    helm_repo(
        name='prometheus-community',
        url='https://prometheus-community.github.io/helm-charts',
    )
    helm_resource('kube-prometheus',
        chart='prometheus-community/kube-prometheus-stack',
        # namespace='monitoring',
        flags=[
            '--set=grafana.enabled=false',
            '--set=prometheus.enabled=false',
            '--wait',
        ]
    )
    k8s_yaml(helm('deploy/charts/opni-monitoring-agent',
        name='opni-monitoring-agent',
        namespace='opni-monitoring-agent',
    ))
else:
    namespace_create('opni-monitoring')
    deploy_cert_manager(version="v1.6.1")

    if "hostname" in settings:
        hostname = settings["hostname"]
    else:
        fail("hostname is required")

    helm_remote('etcd', 
        repo_name='bitnami', 
        repo_url='https://charts.bitnami.com/bitnami',
        namespace='opni-monitoring',
        values="deploy/values/etcd.yaml",
        set=[
            'auth.rbac.rootPassword=tilt',
            'image.tag=3'
        ]
    )
    helm_remote('cortex',
        repo_name='cortex-helm',
        repo_url='https://cortexproject.github.io/cortex-helm-chart',
        namespace='opni-monitoring',
        values=["deploy/values/cortex.yaml", "deploy/custom/cortex.yaml"],
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

    k8s_yaml(listdir('pkg/sdk/crd'))

    k8s_yaml(helm('deploy/charts/opni-monitoring',
        name='opni-monitoring',
        namespace='opni-monitoring',
        values=['deploy/custom/opni-monitoring.yaml'],
    ))

    k8s_resource(workload='opni-gateway', port_forwards=[11090, 11080, 12080])

local_resource('Watch & Compile', 'mage build', 
    deps=['pkg','plugins'], ignore=[
        '**/*.pb.go',
        '**/*.pb.*.go',
        '**/*.swagger.json',
        'pkg/test/mock/*',
        'pkg/sdk/crd/*',
        '**/zz_generated.*',
    ])

if "defaultRegistry" in settings:
    default_registry(settings["defaultRegistry"])

if "dockerfile" in settings:
    dockerfile = settings["dockerfile"]
else:
    dockerfile = 'Dockerfile'

docker_build("kralicky/opni-monitoring", '.', dockerfile=dockerfile, 
    ignore=['mage_output_file.go', 'deploy/'])
