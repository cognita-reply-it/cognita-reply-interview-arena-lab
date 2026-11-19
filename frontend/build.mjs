import { build } from 'esbuild'
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = dirname(fileURLToPath(import.meta.url))
const { kit_version: kitVersion } = JSON.parse(await readFile(resolve(root, '../manifest.json'), 'utf8'))
if (!/^\d+\.\d+\.\d+$/.test(kitVersion)) throw new Error('manifest.json requires a numeric kit_version')
// Alternative paths permit isolated build checks without changing the embedded app.
const entry = resolve(root, process.argv[2] || 'src/main.tsx')
const output = resolve(root, process.argv[3] || '../web/index.html')
const result = await build({
  entryPoints: [entry],
  bundle: true,
  write: false,
  outdir: 'out',
  entryNames: 'app',
  platform: 'browser',
  format: 'iife',
  target: ['es2020'],
  minify: true,
  sourcemap: false,
  legalComments: 'none',
  define: { 'process.env.NODE_ENV': '"production"' },
  logLevel: 'warning',
})
const js = result.outputFiles.find(file => file.path.endsWith('.js'))?.text
const css = result.outputFiles.find(file => file.path.endsWith('.css'))?.text || ''
if (!js) throw new Error('Build did not emit JavaScript')
// HTML parses end tags even inside JavaScript strings and CSS comments.
const safeJs = js.replace(/<\/script/gi, '<\\/script')
const safeCss = css.replace(/<\/style/gi, '<\\/style')
const licenses = await Promise.all([
  ['Stageshow', 'vendor/stageshow/LICENSE'],
  ...['react', 'react-dom', 'lucide-react', 'scheduler', 'loose-envify', 'js-tokens']
    .map(name => [name, `vendor/licenses/${name}.txt`]),
].map(async ([name, path]) => ({ name, license: await readFile(resolve(root, path), 'utf8') })))
const safeLicenses = JSON.stringify(licenses).replace(/<\/script/gi, '<\\/script')
const html = `<!doctype html>\n<html lang="en">\n<head>\n<meta charset="utf-8">\n<meta name="viewport" content="width=device-width, initial-scale=1">\n<meta name="arena-kit-version" content="${kitVersion}">\n<meta name="theme-color" content="#f6f4ef">\n<title>AI Interview Arena</title>\n<style>${safeCss}</style>\n</head>\n<body>\n<noscript>To use AI Interview Arena, enable JavaScript in your browser and reload the page.</noscript>\n<div id="root"></div>\n<script type="application/json" id="third-party-licenses">${safeLicenses}</script>\n<script>${safeJs}</script>\n</body>\n</html>\n`
await mkdir(dirname(output), { recursive: true })
await writeFile(output, html, 'utf8')
console.log(`Built ${output}`)
