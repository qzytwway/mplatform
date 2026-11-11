# Customized GPU Operator chart

MPlatform installs the NVIDIA GPU Operator from the `gpu-operator` chart in the
local chart directory. This directory holds what MPlatform adds to the upstream
chart; it is not a chart by itself. `make gpu-operator-chart` builds the
customized chart:

1. Pulls `gpu-operator` from `https://helm.ngc.nvidia.com/nvidia`.
2. In `values.yaml`, makes the DCGM Exporter ServiceMonitor that the operator
   creates carry the labels the cluster's Prometheus selects, and replaces its
   `instance` label with the node name.
3. Adds `templates/` and `dashboards/` from this directory: a ConfigMap labeled
   `grafana_dashboard` holding the GPU dashboards, which MPlatform's metrics
   page shows for clusters where the chart is installed.
4. Sets the chart version to `<upstream version>-mplatform.1`, checks the
   rendered chart, and packages it into `chart-packages/`, removing the other
   `gpu-operator` packages there so the platform installs this one.

| Variable | Default | Effect |
|---|---|---|
| `GPU_OPERATOR_VERSION` | `v26.7.2` | Upstream chart version |
| `OUT_DIR` | `chart-packages` | Where the package is written |
| `PROMETHEUS_LABELS` | `{release: kube-prometheus-stack}` | Labels of the DCGM Exporter ServiceMonitor, as a YAML flow map. The default matches the kube-prometheus-stack MPlatform installs; set it to the `serviceMonitorSelector` of another Prometheus |

## Dashboards

`dashboards/nvidia-dcgm-exporter.json` is the NVIDIA DCGM Exporter dashboard of
[dcgm-exporter](https://github.com/NVIDIA/dcgm-exporter) 4.8.4 (Apache-2.0),
adapted to what MPlatform's dashboard renderer supports: `timeseries` instead
of `graph` panels, units it can format, and no datasource variable. It uses
the metric names of DCGM Exporter 4.8.x, which GPU Operator v26.7.2 deploys;
DCGM Exporter 4.9.0 renamed them, so update the dashboard together with an
operator that deploys it.
