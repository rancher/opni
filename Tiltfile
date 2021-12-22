load('ext://min_k8s_version', 'min_k8s_version')
load('ext://cert_manager', 'deploy_cert_manager')

settings = read_yaml('tilt-options.yaml', default={})
if "allowedContexts" in settings:
    allow_k8s_contexts(settings["allowedContexts"])

min_k8s_version('1.22')
deploy_cert_manager(version="v1.6.1")

k8s_yaml(kustomize('deploy'))

deps = ['cmd', 'pkg', 'deploy']

local_resource('Watch & Compile', 'mage build', deps=deps)

if "defaultRegistry" in settings:
    default_registry(settings["defaultRegistry"])

if "dockerfile" in settings:
    dockerfile = settings["dockerfile"]
else:
    dockerfile = 'Dockerfile'

docker_build("rancher/opni-gateway", '.', dockerfile=dockerfile)
