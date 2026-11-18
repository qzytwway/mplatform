# mplatform

![Version: 0.18.0](https://img.shields.io/badge/Version-0.18.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: latest](https://img.shields.io/badge/AppVersion-latest-informational?style=flat-square)

A Helm chart for Kubernetes Dashboard - MPlatform

## Installation

### Install from OCI registry

```bash
# Install in kube-system namespace (recommended)
helm install mplatform oci://ghcr.io/qzytwway/charts/mplatform -n kube-system

# Or install in custom namespace
helm install mplatform oci://ghcr.io/qzytwway/charts/mplatform -n my-namespace --create-namespace
```

### Add Helm Repository

```bash
helm repo add mplatform https://charts.mplatform.example.com
helm repo update
```

### Install Chart

```bash
# Install in kube-system namespace (recommended)
helm install mplatform mplatform/mplatform -n kube-system

# Or install in custom namespace
helm install mplatform mplatform/mplatform -n my-namespace --create-namespace
```

### Upgrade Chart

```bash
helm upgrade mplatform oci://ghcr.io/qzytwway/charts/mplatform -n kube-system

# Or use the Helm repository
helm upgrade mplatform mplatform/mplatform -n kube-system
```

### Uninstall Chart

```bash
helm uninstall mplatform -n kube-system
```

### Database

By default the chart runs a PostgreSQL StatefulSet (`db.postgresql.*`) with an
8Gi volume and a password generated on first install, stored in the
`<release>-mplatform-postgresql` Secret. Set `db.dsn` to use an external
PostgreSQL, or `db.type: mysql` with `db.dsn` for MySQL. `db.type: sqlite` is
for trials and development.

Upgrading from 0.17 or earlier with SQLite: the chart refuses to switch an
existing SQLite release to another database, because the new one starts
empty. Set `db.type: sqlite` to keep SQLite, or copy the data into the new
database first and then set `db.migratedFromSqlite: true`. The chart does not
migrate data.

### Chart Values

See [values.yaml](./values.yaml).
