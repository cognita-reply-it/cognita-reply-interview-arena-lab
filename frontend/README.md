# Frontend build

The app's production frontend is bundled into `web/index.html`, which Go embeds in the executable. A running app does not require Node or external assets.

To rebuild, use Node 20 or later and run `npm ci` followed by `npm run typecheck` and `npm run build` in this directory. Dependencies are pinned in the lockfile. The source of truth is `src/`; check in the generated HTML with the corresponding source changes. The build minifies JavaScript and CSS, includes no source maps, and emits a single self-contained HTML file.

For an isolated build check, `node build.mjs <entry> <output>` accepts paths relative to this directory or absolute paths; this does not change the default output. Do not use real participant data as build input or include private evidence in public artifacts.

`vendor/stageshow` contains the subset of Stageshow 1.1.1 kit modules and styles reached by the application’s imports, copied without modification from the Stageshow skill by Benji Taylor. The original kit includes additional parts which are not needed for this build. Their MIT license and original notice are included alongside them. Imports use the component files directly; the illustration-only input and button controls are not used as live form controls.

The output embeds the license texts of its third-party runtime dependencies. Original license files are retained under `vendor/licenses/` and `vendor/stageshow/`.
