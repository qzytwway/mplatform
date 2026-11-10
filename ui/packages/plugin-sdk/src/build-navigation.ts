import { parentPort, workerData } from 'node:worker_threads'
import { createJiti } from 'jiti'

import { getPluginNavigation } from './manifest-navigation.js'
import { validateDefinition } from './validation.js'

const { entryPath, pluginId } = workerData as {
  entryPath: string
  pluginId: string
}
const loader = createJiti(entryPath, { jsx: { runtime: 'automatic' } })

let definition: unknown
try {
  definition = await loader.import(entryPath, { default: true })
} catch (error) {
  throw new Error(
    `Cannot load plugin config for navigation metadata: ${error instanceof Error ? error.message : String(error)}. Keep browser APIs, CSS and page dependencies in lazy imports.`,
    { cause: error }
  )
}
validateDefinition(pluginId, definition)
parentPort!.postMessage(getPluginNavigation(definition))
