#!/usr/bin/env bash
# Removes what kubeadm init/join left behind on this node.
#   reset.sh retry                   only when a previous attempt left state (before init/join)
#   reset.sh delete <id> <cluster>   when deleting the cluster; also stops kubelet. Does
#                                    nothing unless the node carries the ownership marker
#                                    of that installation.
set -uo pipefail
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
export LC_ALL=C

marker=/var/lib/mplatform/owner
mode=${1:-retry}
if [ "$mode" = delete ]; then
  expected="provision=${2:-}
cluster=${3:-}"
  if [ ! -f "$marker" ]; then
    echo "No MPlatform ownership marker on this node; leaving it untouched"
    exit 0
  fi
  if [ "$(cat "$marker")" != "$expected" ]; then
    echo "This node was prepared by another installation ($(tr '\n' ' ' <"$marker")); leaving it untouched"
    exit 0
  fi
elif [ "$mode" = retry ]; then
  found=""
  for path in /etc/kubernetes/kubelet.conf /etc/kubernetes/bootstrap-kubelet.conf /etc/kubernetes/admin.conf \
    /etc/kubernetes/manifests/kube-apiserver.yaml /etc/kubernetes/manifests/kube-vip.yaml /var/lib/etcd/member; do
    [ -e "$path" ] && found=1
  done
  if [ -z "$found" ]; then
    exit 0
  fi
  echo "==> Cleaning up a previous attempt"
else
  echo "unknown mode $mode"
  exit 1
fi

if command -v kubeadm >/dev/null 2>&1; then
  kubeadm reset -f --cri-socket unix:///run/containerd/containerd.sock
fi
rm -f /etc/kubernetes/manifests/kube-vip.yaml
rm -rf /var/lib/etcd /etc/cni/net.d
ip link delete vxlan.calico >/dev/null 2>&1 || true

if [ "$mode" = delete ]; then
  systemctl disable --now kubelet.service >/dev/null 2>&1 || true
  rm -rf /etc/kubernetes /var/lib/calico /var/run/calico
  rm -f "$marker"
fi
exit 0
