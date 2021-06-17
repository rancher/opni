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

do_install_opni() {
    if [ -z "${INSTALL_RKE2_ARTIFACT_PATH}" ]; then
        verify_downloader curl || verify_downloader wget || fatal "can not find curl or wget for downloading files"
    fi
    do_install_rke2
    #download "/usr/local/bin/opnictl" "${OPNICTL_URL}"
    info "Installing Opni Manager"
    KUBECONFIG=/etc/rancher/rke2/rke2.yaml opnictl install
    sleep 10
    info "Installing Opni Quickstart"
    KUBECONFIG=/etc/rancher/rke2/rke2.yaml opnictl create demo --quickstart --timeout 10m
    info "Waiting for controlplane logs to stabilize"
    sleep 30
    info "Generating controlplane anomalies"
    inject_anomaly
}

do_install_opni
exit 0