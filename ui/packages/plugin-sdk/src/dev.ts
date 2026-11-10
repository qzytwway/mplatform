import { parseArgs } from 'node:util'
import { build, preview, type PreviewServer, type Rollup } from 'vite'

const { values } = parseArgs({
  args: process.argv.slice(3),
  options: {
    host: { type: 'string', default: 'localhost' },
    port: { type: 'string', default: '5174' },
  },
})
const port = Number(values.port)
if (!Number.isInteger(port) || port < 1 || port > 65535) {
  throw new Error('Port must be an integer between 1 and 65535')
}

const watcher = (await build({
  mode: 'development',
  logLevel: 'warn',
  build: { watch: {}, sourcemap: true, minify: false },
})) as Rollup.RollupWatcher

let server: PreviewServer | undefined
let stopping = false
async function stop() {
  if (stopping) return
  stopping = true
  await watcher.close()
  await server?.close()
  process.exit(0)
}
process.on('SIGINT', () => void stop())
process.on('SIGTERM', () => void stop())

await new Promise<void>((resolve) => {
  watcher.on('event', (event) => {
    if (event.code === 'BUNDLE_END') {
      if (server)
        console.log('Plugin rebuilt. Refresh MPlatform to apply changes.')
      resolve()
    }
  })
})

try {
  server = await preview({
    mode: 'development',
    appType: 'custom',
    preview: {
      host: values.host,
      port,
      strictPort: true,
      open: false,
      cors: true,
      headers: { 'Cache-Control': 'no-store' },
    },
  })
} catch (error) {
  await watcher.close()
  throw error
}

const urls = server.resolvedUrls!
for (const url of [...urls.local, ...urls.network]) {
  console.log(`Plugin URL: ${new URL('plugin.json', url).href}`)
}
console.log(
  'Set PLUGIN_DEV_URL when starting MPlatform. Refresh MPlatform after each rebuild.'
)
