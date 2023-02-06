#!/usr/bin/env bash

export run_garden="echo 'yes' | ./scripts/remote-fly ci/garden/task.yml"
export run_guardian="echo 'yes' | ./scripts/remote-fly ci/guardian/task.yml -i tar-out=${TAR_OUT}"
export run_guardian_containerd="echo 'yes' | CONTAINERD_ENABLED=true ./scripts/remote-fly ci/guardian/task.yml -i tar-out=${TAR_OUT}"
export run_guardian_nerdful="echo 'yes' | CONTAINERD_ENABLED=true CONTAINERD_FOR_PROCESSES_ENABLED=true ./scripts/remote-fly ci/guardian/tas.yml -i tar-out=${TAR_OUT}"
export run_guardian_throttled="echo 'yes' | CONTAINERD_ENABLED=true CPU_THROTTLING_ENABLED=true ./scripts/remote-fly ci/guardian/tas.yml -i tar-out=${TAR_OUT}"
export run_guardian_standalone="echo 'yes' | ./scripts/remote-fly ci/guardian-standalone/task.yml -i tar-out=${TAR_OUT} -i iptables-out=${IPTABLES_OUT}"
