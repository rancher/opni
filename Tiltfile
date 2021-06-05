bin_path = os.getcwd()+'/bin'
if not os.environ.get('PATH').endswith(bin_path):
  os.environ.update({'PATH': os.environ.get('PATH')+':'+bin_path})

load('ext://kubebuilder', 'kubebuilder')
load('ext://min_k8s_version', 'min_k8s_version')

min_k8s_version('1.20')
kubebuilder('demo', 'opni.io', 'v1alpha1', 'OpniDemo', 'joekralicky/opni-manager')
