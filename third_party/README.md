# Third-party components

This directory accompanies the AI Interview Arena source code and package. Keep these notices when redistributing the package. Binary and model versions and checksums are recorded in [`manifest.json`](../manifest.json). For the model’s provenance and the limits of the available information, see [`docs/MODEL.md`](../docs/MODEL.md).

| Component | Version and provenance | Retained text |
| --- | --- | --- |
| llama.cpp | b11233, commit `d77dd0806dc26fc418273ef99f88da11239ca41b` | `LLAMA-LICENSE.txt` (MIT) |
| nlohmann/json, bundled by llama.cpp | File from the same commit | `llama-jsonhpp-LICENSE.txt` |
| cpp-httplib, bundled by llama.cpp | File from the same commit | `llama-cpp-httplib-LICENSE.txt` |
| Hashing libraries vendored by llama.cpp | Files from the same commit | `llama-rotate-bits-LICENSE.txt`, `llama-sha1-NOTICE.txt`, `llama-sha256-LICENSE.txt`, `llama-xxhash-LICENSE.txt` |
| miniaudio, stb_image, sheredom/subprocess | Headers from the same commit, bundled by libmtmd | `llama-miniaudio-LICENSE.txt`, `llama-stb-image-LICENSE.txt`, `llama-sheredom-subprocess-LICENSE.txt` |
| llamafile/sgemm kernel | Copyright Mozilla Foundation, bundled in the CPU backend | `llama-llamafile-sgemm-LICENSE.txt` (MIT) |
| BoringSSL and fiat-crypto | BoringSSL `0.20260903.0`, commit `0ce57bbf06a35864fa39e3b9346d41e833683383`, statically linked into the upstream macOS build | `llama-boringssl-LICENSE.txt`, `llama-boringssl-fiat-LICENSE.txt`, `llama-boringssl-fiat-AUTHORS.txt` |
| Upstream UI embedded in the engine | b11233 assets. The kit disables this UI. Its code remains in the redistributed runtime. | `llama-ui-NOTICES.txt`, `llama-ui-sources.json`, `llama-ui-big-integer-LICENSE.txt`, `llama-ui-nerdamer-prime-LICENSE.txt` |
| Qwen3.5-4B | Qwen, reference revision `851bf6e806efd8d0a36b00ddf55e13ccb7b8cd0a` | `QWEN-LICENSE.txt` (Apache 2.0), `QWEN-MODEL-CARD.md` |
| Qwen3.5-4B GGUF Q4_K_M | Unsloth, revision `e87f176479d0855a907a41277aca2f8ee7a09523` | `UNSLOTH-MODEL-CARD.md`, which points to the Qwen license |
| Go runtime and standard library | Go 1.27.1, compiled into the app executable | `GO-LICENSE.txt` (BSD 3-Clause) |
| Go packages vendored in the standard library | `golang.org/x/crypto`, `x/net`, `x/sys`, `x/text` in the Go 1.27.1 toolchain | `go-vendor-*-LICENSE.txt` |

The llama.cpp macOS archive contains a 21-line MIT `LICENSE` for llama.cpp itself. The dependency notices from the source of the same release are kept separately. The app uses the prebuilt upstream executable, and macOS provides the system libraries.

The Ubuntu x64 and Windows x64 kits preserve the corresponding official llama.cpp b11233 CPU archives. Each kit records the original archive URL and SHA-256 in its own manifest. The Windows runtime includes `libomp.dll` and its upstream `LICENSE-LLVM-OpenMP` notice under `runtime/windows-x64/llama-b11233/`.

License files and model cards are byte-for-byte copies of their sources. For notices that appear only in headers, the original blocks are retained verbatim. The full source hashes and extracted line ranges are recorded in [`sources.json`](sources.json). The Go notices come from the pinned local toolchain. Participants can run the kit without installing that toolchain.

`llama-ui-NOTICES.txt` conservatively collects notices for 328 packages, covering the browser dependencies and their non-optional dependencies according to the b11233 lockfile. The collection includes some components that the bundler may remove. `llama-ui-sources.json` records versions, npm archive URLs, integrity checks against the lockfile, and hashes for the individual notice texts. The collection was produced without installing or running any npm packages. It includes the font, icon, and PDF library notices from their respective archives. The mdsvex/prism-svelte build preprocessor is outside the scope of this collection because it is used only at build time.

Inspection of the macOS runtime found dynamic links exclusively to libraries in the archive and system frameworks. The inspection found zero Homebrew paths. The BoringSSL version comes from the pinned upstream CMake configuration, and inspection confirmed that the binary contains the statically linked code. The inspected BoringSSL revision has no separate NOTICE file. The kit ships the official runtime as released.

The model cards are upstream documentation preserved for attribution and verification. They may contain links, remote images, or examples using cloud services. The kit has no dependency on those resources and the workshop does not use those examples as instructions. AI Interview Arena runs inference through the local engine. The app UI does not load the model cards.

These notices neither grant nor change a license for the repository’s original code. This work keeps model weights out of Git and does not publish them.

## Qwen3-4B, the only alternative tested

The documented alternative is `Qwen/Qwen3-4B-GGUF` at revision `bc640142c66e1fdd12af0bd68f40445458f3869b`, file `Qwen3-4B-Q4_K_M.gguf`. The official Qwen account publishes this GGUF under Apache 2.0 with the attribution Copyright 2025 Alibaba Cloud.

`QWEN3-LICENSE.txt` and `QWEN3-GGUF-MODEL-CARD.md` are copies from that revision. `QWEN3-BASE-MODEL-CARD.md` preserves the documentation for `Qwen/Qwen3-4B` at revision `1cfa9a7208912126459214e8b04321603b3df60c`. The model card names the base model and leaves its exact conversion input commit unspecified. The revision identifies the documentation only.

These notices preserve the documentation for the only alternative tested. The observed factual fidelity problems persisted, so the kit kept Qwen3.5-4B as the active model, as specified in the root manifest. Historical test reports are retained locally only. The main package excludes the Qwen3 weights used during testing.
