#!/bin/sh

set -e

# info logs the given argument at info log level.
info() {
    echo "[INFO] " "$@"
}

# warn logs the given argument at warn log level.
warn() {
    echo "[WARN] " "$@" >&2
}

# fatal logs the given argument at fatal log level.
fatal() {
    echo "[ERROR] " "$@" >&2
    if [ -n "${SUFFIX}" ]; then
        echo "[ALT] Please visit 'https://github.com/rancher/rke2/releases' directly and download the latest rke2.${SUFFIX}.tar.gz" >&2
    fi
    exit 1
}

verify_downloader() {
    cmd="$(command -v "${1}")"
    if [ -z "${cmd}" ]; then
        return 1
    fi
    if [ ! -x "${cmd}" ]; then
        return 1
    fi

    # Set verified executable as our downloader program and return success
    DOWNLOADER=${cmd}
    return 0
}

download() {
    if [ $# -ne 2 ]; then
        fatal "download needs exactly 2 arguments"
    fi

    case ${DOWNLOADER} in
    *curl)
        curl -o "$1" -fsSL "$2"
        ;;
    *wget)
        wget -qO "$1" "$2"
        ;;
    *)
        fatal "downloader executable not supported: '${DOWNLOADER}'"
        ;;
    esac

    # Abort if download command failed
    if [ $? -ne 0 ]; then
        fatal "download failed"
    fi
}

do_install_rke2() {
    curl -sfL https://get.rke2.io | sh -
    systemctl enable rke2-server.service > /dev/null 2>&1
    systemctl start rke2-server.service
    until /var/lib/rancher/rke2/bin/kubectl --kubeconfig /etc/rancher/rke2/rke2.yaml get nodes > /dev/null 2>&1
    do
        info "Waiting for RKE2 cluster to be active"
        sleep 10
    done
}

inject_anomaly() {
    cat <<EOF | /var/lib/rancher/rke2/bin/kubectl --kubeconfig /etc/rancher/rke2/rke2.yaml apply -f -
    apiVersion: batch/v1
    kind: Job
    metadata:
      name: test-job
    spec:
      completions: 50
      parallelism: 50
      template:
        spec:
          containers:
            - name: test
              image: busybox
              command:
              - /bin/ls
          affinity:
            nodeAffinity:
              requiredDuringSchedulingIgnoredDuringExecution:
                nodeSelectorTerms:
                - matchExpressions:
                  - key: nonexistent
                    operator: Exists
          restartPolicy: Never
EOF
}

get_user_anomaly_input() {
  info "Waiting to inject anomaly; press enter to inject anomaly or type quit to exit"
  read k
  if [ "$k" = "quit" ]
  then
    info "Exiting quickstart script"
  else
    info "Injecting Anomaly"
    inject_anomaly
  fi
}

do_install_opni() {
    if [ -z "${INSTALL_RKE2_ARTIFACT_PATH}" ]; then
        verify_downloader curl || verify_downloader wget || fatal "can not find curl or wget for downloading files"
    fi
    do_install_rke2
    sleep 5
    download "/usr/local/bin/opnictl" "https://github.com/rancher/opni/releases/download/v0.1.1/opnictl_linux-amd64"
    chmod +x /usr/local/bin/opnictl
    info "Installing Opni Manager"
    KUBECONFIG=/etc/rancher/rke2/rke2.yaml opnictl install
    sleep 10
    info "Installing Opni Quickstart"
    KUBECONFIG=/etc/rancher/rke2/rke2.yaml opnictl create demo --quickstart --timeout 10m
    NODEPORT=$(/var/lib/rancher/rke2/bin/kubectl --kubeconfig /etc/rancher/rke2/rke2.yaml -n opni-demo get -o jsonpath="{.spec.ports[0].nodePort}" services opendistro-es-kibana-svc)
    echo "The opni kibana dashboard is listening on port ${NODEPORT}"
    echo "Navigate to http://<external_ip>:${NODEPORT} and login with the default admin user to view the dashboards"
}

do_install_opni
get_user_anomaly_input
exit 0