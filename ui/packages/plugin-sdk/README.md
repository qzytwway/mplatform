# MPlatform Plugin SDK

Build React plugins for MPlatform with `@mplatform/plugin-sdk`. Plugins can add pages and sidebar menus, work with Kubernetes resources and CRDs, and reuse MPlatform's UI components, authentication, cluster selection, and query cache.

A plugin runs inside MPlatform and uses the signed-in user's permissions. The SDK includes TypeScript definitions, a Vite configuration helper (`@mplatform/plugin-sdk/vite`), and the `mplatform-plugin` CLI for local development and packaging.

Requirements: Node.js `^20.19.0 || >=22.12.0` and a MPlatform instance matching the plugin's `engines.mplatform` version range (default: `>=0.16.0`).

```sh
pnpm run build             # build dist/
mplatform-plugin dev       # rebuild on change and serve plugin.json
mplatform-plugin pack      # create <plugin-id>-<version>.tar.gz from dist/
```

Upload the archive from **Plugin management** in the avatar menu.
