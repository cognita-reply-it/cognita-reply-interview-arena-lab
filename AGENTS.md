# Repository instructions

## Preserve local work

- Never permanently delete a file or directory. All filesystem removals must go to Trash or the Recycle Bin.
- Do not run destructive Git operations, including `git clean`, `git reset --hard`, bulk checkout or bulk restore.
- Preserve existing work, local model files and test evidence. Use a fresh, dedicated test directory for a clean-install test; record exactly which caches were or were not reused.
- Do not push, publish releases or upload artifacts unless explicitly authorized by the user.

## Privacy is a publication requirement

- Never commit or publish information attributable to the user's computer: absolute local paths, home-directory names, hostnames, local account identifiers, personal email addresses copied from the environment, machine identifiers, hardware inventories, OS fingerprints, process IDs, private network addresses, local configuration or credentials.
- Keep local test harnesses, actual test runs, transcripts, screenshots, logs, performance measurements and reports under `.local-tests/`, excluded from Git. Previously generated evidence may remain in dedicated ignored locations, including historical files under `evaluation/`, `output/` and `packages/`; preserve a private archive before editing associated documentation. Existing evidence must remain available to the user.
- Public files may contain fictional teaching cases, generic operating-system requirements, relative paths, portable commands and upstream dependency provenance. Do not copy the local machine's details into those examples.
- Review ZIPs and other generated artifacts too: exclusions in the source tree do not protect a package that embeds private reports or build paths. Use explicit packaging allowlists and reproducible builds without local source paths or VCS metadata.
- `.gitignore` does not untrack files already committed and does not erase history. Before any authorized push, inspect every commit that would be sent, not only the current files. Block a push containing private evidence or a previously identified private commit.
- Do not rewrite or discard existing history to hide a privacy issue. Preserve it locally and prepare a separate clean publication branch when necessary; do not push the private history.
- Test reports must explain purpose, context, input, expected behavior, actual output, timings and limitations. Separate technical completion from manually judged feedback quality.
