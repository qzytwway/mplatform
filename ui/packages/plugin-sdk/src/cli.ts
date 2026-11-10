import { createHash } from 'node:crypto'
import { lstat, readdir, readFile } from 'node:fs/promises'
import { relative, resolve } from 'node:path'
import { create } from 'tar'

import { validateManifest } from './validation.js'

const [command, directory = 'dist', output] = process.argv.slice(2)

if (command === 'dev') {
  await import('./dev.js')
} else if (command === 'pack') {
  const root = resolve(directory)
  const manifest: unknown = JSON.parse(
    await readFile(resolve(root, 'plugin.json'), 'utf8')
  )
  validateManifest(manifest)

  const themeStyles = (manifest.themes ?? []).flatMap((theme) => theme.styles)
  for (const path of [
    manifest.entry,
    ...(manifest.styles ?? []),
    ...themeStyles,
  ]) {
    if (!(await lstat(resolve(root, path))).isFile()) {
      throw new Error(
        `Plugin asset must be a regular file inside dist: ${path}`
      )
    }
  }
  const files = await readdir(root, { recursive: true })
  for (const file of files) {
    const stat = await lstat(resolve(root, file))
    if (!stat.isFile() && !stat.isDirectory())
      throw new Error(`Unsupported package entry: ${file}`)
  }

  const archive = resolve(output ?? `${manifest.id}-${manifest.version}.tar.gz`)
  if (!relative(root, archive).startsWith('../'))
    throw new Error('Archive output must be outside the dist directory')
  await create(
    { cwd: root, file: archive, gzip: true, portable: true },
    (await readdir(root)).sort()
  )
  const checksum = createHash('sha256')
    .update(await readFile(archive))
    .digest('hex')
  console.log(`${archive}\nsha256: ${checksum}`)
} else {
  console.error(`Usage:
  mplatform-plugin dev [--host localhost] [--port 5174]
  mplatform-plugin pack [dist-directory] [output.tar.gz]`)
  process.exitCode = 1
}
