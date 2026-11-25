# mplatform

![Version: 0.18.0](https://img.shields.io/badge/Version-0.18.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: latest](https://img.shields.io/badge/AppVersion-latest-informational?style=flat-square)

A Helm chart for MPlatform

## Installation

### Prepare a values file

Generate your own keys and keep the app store's charts on a persistent volume.
Keep this file and pass it to every install and upgrade: without it the keys
fall back to the built-in defaults, and saved cluster credentials can no longer
be decrypted.

```bash
cat > mplatform-values.yaml <<EOF
encryptKey: "$(openssl rand -hex 32)"
jwtSecret: "$(openssl rand -hex 32)"
chartStore:
  persistence:
    pvc:
      enabled: true
EOF
```

### Install Chart

```bash
helm install mplatform oci://ghcr.io/qzytwway/charts/mplatform \
  --namespace mplatform-system --create-namespace \
  -f mplatform-values.yaml
```

### Upgrade Chart

```bash
helm upgrade mplatform oci://ghcr.io/qzytwway/charts/mplatform \
  --namespace mplatform-system -f mplatform-values.yaml
```

### Uninstall Chart

```bash
helm uninstall mplatform --namespace mplatform-system
```

### Database

By default the chart runs a PostgreSQL StatefulSet (`db.postgresql.*`) with an
8Gi volume and a password generated on first install, stored in the
`mplatform-postgresql` Secret. Set `db.dsn` to use an external PostgreSQL, or
`db.type: mysql` with `db.dsn` for MySQL. `db.type: sqlite` is for trials and
development.

### Chart Values

See [values.yaml](./values.yaml).
