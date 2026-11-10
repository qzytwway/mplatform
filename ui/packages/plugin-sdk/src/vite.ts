import { existsSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { Worker } from 'node:worker_threads'
import { federation } from '@module-federation/vite'
import type { Plugin, UserConfig } from 'vite'

import { readPluginManifest } from './build-manifest.js'
import type { PluginManifest } from './index.js'

export const sharedModules = [
  'react',
  'react/jsx-runtime',
  'react/jsx-dev-runtime',
  'react-dom',
  'react-dom/client',
  'react-router-dom',
  '@tanstack/react-query',
  'react-i18next',
  '@mplatform/plugin-sdk',
  '@mplatform/plugin-sdk/resources',
  '@mplatform/plugin-sdk/ui',
  '@mplatform/plugin-sdk/navigation',
  '@mplatform/plugin-sdk/i18n',
  '@mplatform/plugin-sdk/api',
  '@mplatform/plugin-sdk/observability',
  '@mplatform/plugin-sdk/hooks',
] as const

export interface MPlatformPluginOptions {
  entry?: string
}

interface FederationStatsEntry {
  assets: { css: { sync: string[]; async: string[] } }
}

export function mplatformPlugin({
  entry = './plugin.config.tsx',
}: MPlatformPluginOptions = {}): UserConfig {
  const entryPath = resolve(entry)
  const packagePath = resolve('package.json')
  const readmePath = resolve('README.md')
  let manifest: PluginManifest = readPluginManifest(packagePath)
  const { id, version } = manifest

  const packageManifest: Plugin = {
    name: 'mplatform-plugin-manifest',
    enforce: 'post',
    async buildStart() {
      this.addWatchFile(entryPath)
      this.addWatchFile(packagePath)
      this.addWatchFile(readmePath)
      manifest = readPluginManifest(packagePath)
      if (manifest.id !== id || manifest.version !== version)
        this.error('Restart the plugin build after changing its id or version')
      // A fresh worker also resets native ESM caches for imported config constants during watch builds.
      const navigation = await new Promise<Partial<PluginManifest>>(
        (resolve, reject) => {
          const worker = new Worker(
            new URL('./build-navigation.js', import.meta.url),
            { workerData: { entryPath, pluginId: id } }
          )
          worker.once('message', resolve)
          worker.once('error', reject)
        }
      )
      manifest = { ...manifest, ...navigation }
    },
    generateBundle(_options, bundle) {
      if (existsSync(readmePath)) {
        this.emitFile({
          type: 'asset',
          fileName: 'README.md',
          source: readFileSync(readmePath),
        })
      }
      const styles = Object.values(bundle)
        .filter(
          (asset) => asset.type === 'asset' && asset.fileName.endsWith('.css')
        )
        .map((asset) => asset.fileName)
      this.emitFile({
        type: 'asset',
        fileName: 'plugin.json',
        source: JSON.stringify({ ...manifest, styles }, null, 2),
      })
    },
  }

  return {
    base: './',
    plugins: [
      federation({
        name: `mplatform_plugin_${manifest.id.replaceAll('-', '_')}_${manifest.version.replaceAll(/[^a-zA-Z0-9_]/g, '_')}`,
        filename: 'remoteEntry.js',
        publicPath: 'auto',
        manifest: {
          additionalData({ stats }) {
            // The host owns stylesheet attachment and removal.
            for (const entries of [stats.exposes, stats.shared]) {
              if (!Array.isArray(entries)) continue
              for (const entry of entries as FederationStatsEntry[]) {
                entry.assets.css = { sync: [], async: [] }
              }
            }
            return stats
          },
        },
        experiments: { externalRuntime: true },
        dts: false,
        bundleAllCSS: false,
        exposes: { './plugin': entryPath },
        shared: Object.fromEntries(
          sharedModules.map((name) => [
            name,
            {
              singleton: true,
              import: false,
              requiredVersion: false,
            },
          ])
        ),
      }),
      packageManifest,
    ],
    build: {
      target: 'es2022',
      cssCodeSplit: false,
      modulePreload: true,
      rollupOptions: { input: entryPath },
    },
  }
}
