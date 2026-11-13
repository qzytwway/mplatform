# Third-party chart packages

This directory is MPlatform's local chart repository (`CHART_DIR`). It holds
the chart versions the platform is verified with, so a platform run from this
repository can install its components without downloading charts. The
packages are distributed under their own licenses, not MPlatform's.

All packages except `gpu-operator` are the upstream releases, unmodified;
their SHA-256 checksums match the packages the upstream repositories serve.

| Package | Source | License | SHA-256 |
| --- | --- | --- | --- |
| `kube-prometheus-stack-93.1.1.tgz` | [prometheus-community/helm-charts](https://github.com/prometheus-community/helm-charts), `https://prometheus-community.github.io/helm-charts` | Apache-2.0 | `b34c1267e469ef1ab5d39028d682d6b106eca7e98231e3bf4c0faf9922875ae3` |
| `victoria-logs-single-0.13.12.tgz` | [VictoriaMetrics/helm-charts](https://github.com/VictoriaMetrics/helm-charts), `https://victoriametrics.github.io/helm-charts/` | Apache-2.0 | `8c1342d05d2a2c1c65d56995c4f489a1f19f11520f12408963d4f57aaacca2a6` |
| `vector-0.61.0.tgz` | [vectordotdev/helm-charts](https://github.com/vectordotdev/helm-charts), `https://helm.vector.dev` | MPL-2.0 | `ea356d46d8022d0f1f24e871b770cf7f24b7d310566bd62920dc2238f99601d5` |
| `lws-1.0.0.tgz` (chart version `v1.0.0`) | [kubernetes-sigs/lws](https://github.com/kubernetes-sigs/lws), `oci://registry.k8s.io/lws/charts/lws` | Apache-2.0 | `ea6cff664078688aeed8727afcb2e9cef481f3651919a3b49c0f6dfc50c9627b` |
| `hami-2.11.0.tgz` | [Project-HAMi/HAMi](https://github.com/Project-HAMi/HAMi), `https://project-hami.github.io/HAMi/` | Apache-2.0 | `2530d25ba1a953a5ffa8070c308a373f756c6108c5bb39fce49943f614dd9315` |
| `higress-core-2.2.6.tgz` | [alibaba/higress](https://github.com/alibaba/higress), `https://higress.io/helm-charts` | Apache-2.0 | `14d34483346ce52fb8bbd0b982264b25ebc5aa4000cf2ec6527e2b5328f49ffc` |
| `gpu-operator-v26.7.2-mplatform.1.tgz` | [NVIDIA/gpu-operator](https://github.com/NVIDIA/gpu-operator) v26.7.2, `https://helm.ngc.nvidia.com/nvidia`, modified | Apache-2.0 | `59bae387841e2f370eb2ebe017cca2150cc78a2c1e99a55bdd4d04de686a23fe` |

`gpu-operator-v26.7.2-mplatform.1.tgz` is built by `make gpu-operator-chart`
from the upstream chart. It changes the DCGM Exporter ServiceMonitor values
and adds a GPU dashboard derived from the dcgm-exporter dashboard (Apache-2.0);
[charts/addons/gpu-operator](../charts/addons/gpu-operator/README.md) lists
the changes.

The license texts are at <https://www.apache.org/licenses/LICENSE-2.0> and
<https://mozilla.org/MPL/2.0/>.

When a package is replaced with another version, update this table, the
tested versions in the code and the component table of the README together.
