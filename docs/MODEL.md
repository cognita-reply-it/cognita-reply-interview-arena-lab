# Local model and inference engine

This release uses **Qwen3.5-4B** in **Unsloth’s GGUF Q4_K_M** quantization, running on **llama.cpp b11233**. The model and engine are separate components. Changing either one requires rerunning validation. The versions are pinned in [`manifest.json`](../manifest.json). The sources were consulted on September 28, 2026.

## Pinned artifacts

| Component | Identity |
| --- | --- |
| Original model | `Qwen/Qwen3.5-4B`, reference revision `851bf6e806efd8d0a36b00ddf55e13ccb7b8cd0a` |
| Quantized distribution | `unsloth/Qwen3.5-4B-GGUF`, revision `e87f176479d0855a907a41277aca2f8ee7a09523` |
| File | `Qwen3.5-4B-Q4_K_M.gguf` |
| GGUF size | 2,740,937,888 bytes, approximately 2.55 GiB |
| GGUF SHA256 | `00fe7986ff5f6b463e62455821146049db6f9313603938a70800d1fb69ef11a4` |
| Inference engine | `llama.cpp b11233`, commit `d77dd0806dc26fc418273ef99f88da11239ca41b` |
| macOS archive | `llama-b11233-bin-macos-arm64.tar.gz`, 11,757,731 bytes |
| macOS archive SHA256 | `0daa6915a5b2d9eceafe4671566382c65412c42385db445a9279c4052b89abb4` |
| Ubuntu x64 archive and SHA256 | `llama-b11233-bin-ubuntu-x64.tar.gz` · `21773f64c04f30645f04ecb004b927dfea209c7e3c4a174d632d35c2311607a3` |
| Windows x64 CPU archive and SHA256 | `llama-b11233-bin-win-cpu-x64.zip` · `bb7020dce1ba359a402c6520a8bcbad561d3aecb9fa40d6fd15a50d3e4d8756b` |
| App build toolchain | Go `1.27.1`, required only when building the app |

Download links

- [GGUF at the pinned revision](https://huggingface.co/unsloth/Qwen3.5-4B-GGUF/resolve/e87f176479d0855a907a41277aca2f8ee7a09523/Qwen3.5-4B-Q4_K_M.gguf)
- [Official llama.cpp b11233 archive for macOS arm64](https://github.com/ggml-org/llama.cpp/releases/download/b11233/llama-b11233-bin-macos-arm64.tar.gz)

Use the SHA256 checksum of the LFS blob published by Hugging Face to verify the model. The CDN returns a separate ETag. An HTTP Range verification request returned `206 Partial Content`, the expected size, and a version 3 `GGUF` header. Verify the SHA256 of the complete file after transfer.

## Provenance and licensing

Qwen publishes the original model under the **Apache 2.0** license. **Unsloth** publishes the selected GGUF. Its model card identifies `Qwen/Qwen3.5-4B` as the base model and points to the same Apache 2.0 license.

The GGUF revision and hash identify the exact file distributed. The model card does not specify the original model commit used for conversion. **The Qwen revision above identifies only the official documentation consulted.** We did not reproduce the quantization from the original weights. Q4_K_M is a mixed quantization in the 4-bit family and uses other precisions for some tensors. The workshop uses text only, so it runs without the `mmproj` multimodal projector.

llama.cpp uses the **MIT** license. The Go runtime uses the **BSD 3-Clause** license. Their license texts, the vendored dependency notices, and the original model cards are stored in [`third_party/`](../third_party/README.md). Licensing for the repository’s own code is independent of these licenses.

## Chat template and reasoning controls

The llama.cpp b11233 source supports the `qwen35` architecture. The metadata of the downloaded GGUF confirms the model’s identity (`general.architecture=qwen35`, `general.name=Qwen3.5-4B`). The kit enables `--jinja` to use the Jinja template embedded in the GGUF as shipped.

**The Unsloth GGUF template and the original Qwen template use different defaults.** Qwen’s original model card and template describe thinking as enabled by default. In the GGUF’s `tokenizer.chat_template` field, the branch that opens `<think>` requires `enable_thinking is defined and enable_thinking is true`. In all other cases, the template inserts `<think>\n\n</think>\n\n`. This difference was verified against the local file identified by the checksum above. The app always sends `chat_template_kwargs.enable_thinking` as an explicit boolean. It sends `true` to turn reasoning on and `false` to turn it off.

Qwen3.5 does not officially support the `/think` and `/nothink` text switches. The comparison uses the template parameters. Hiding reasoning only changes what is displayed. Turning reasoning off requires the template control.

The default configuration uses **512 reasoning tokens and 900 total tokens**. These are settings chosen for the kit. Their effect on quality requires validation. The total limit must leave room for the final feedback. The kit distributes Qwen3.5-4B only. The retained notices document the provenance of Qwen3-4B, which was evaluated as an alternative.

The actual comparisons and their interpretation are kept locally as test materials. To compare instructions and budgets while assessing completion, time, and quality independently, see the [validation method](VALIDATION.md).

You can change both budgets when launching the app, without rebuilding it.

```sh
./ai-interview-arena --reasoning-budget 512 --max-tokens 900
```

The app launches the engine with `--jinja --reasoning-format deepseek --reasoning-budget` and the chosen budget. It also sends the following controls with each request.

```json
{
  "chat_template_kwargs": {"enable_thinking": true},
  "reasoning_budget_tokens": 512,
  "max_tokens": 900
}
```

The JSON field is `reasoning_budget_tokens`. Version b11233 also accepts `thinking_budget_tokens` as an alias. The server does not recognize `reasoning_budget` as a JSON field for this setting. To disable reasoning, use `enable_thinking: false`. You can also set the reasoning budget to zero. The app accepts a reasoning budget from 0 to 2048 tokens. The total budget must allow at least 256 additional tokens and can be at most 4096 tokens. A complete reply has a 60-second time limit on macOS. The Ubuntu and Windows CPU kits allow 5 minutes by default. Set `--reply-timeout 60s` to apply the classroom timing target to CPU runs.

When the reasoning budget runs out, the sampler forces the reasoning closing sequence and continues generating the final response. **It waits only for any in-progress UTF-8 sequence to complete. The cutoff can occur mid-sentence or mid-reasoning.** A properly closed tag confirms the closing sequence was emitted. Completion of the model’s fact check still needs to be assessed from the final feedback.

The optional `reasoning_budget_message` control (engine CLI flag `--reasoning-budget-message`) inserts forced text immediately before the closing tag, only when the budget limit is reached. The model receives zero additional free-form reasoning tokens. The kit leaves this setting unset. The sources describe how the control works, with no documented message that guarantees greater factual fidelity for Qwen3.5-4B. The kit leaves experimental options for early warnings and post-budget grace periods unused.

The overall `max_tokens` limit includes reasoning, any forced text, and the final response. Leave enough room for the feedback. With `--reasoning-format deepseek`, the engine returns reasoning separately in `reasoning_content`. The coach displays and retains only `content`, the final feedback. A larger budget can give the model enough tokens to complete a check. Reassess the feedback against the case facts to see whether any hallucination was corrected.

Count a run as unsuccessful if it ends with `finish_reason: "length"`, yields no final feedback, or times out. Measure the time from submission to the end of the feedback, including prompt processing, reasoning, and final generation.

For general tasks with reasoning enabled, Qwen’s model card suggests `temperature=1.0`, `top_p=0.95`, `top_k=20`, `min_p=0`, `presence_penalty=1.5`, and `repetition_penalty=1.0`. With reasoning disabled, it suggests `temperature=0.7` and `top_p=0.8` and keeps the other values the same. The kit can use more conservative settings based on workshop testing. Report the actual settings and seeds for every comparison, and specify whether you held sampling constant or compared the two recommendations. The short budgets used in the workshop are application choices that require validation.

## Acceleration and validation limits

Each package contains the native engine and libraries for its target operating system. The app is built with Go 1.27.1 and the settings `CGO_ENABLED=0`, `-trimpath`, and `-buildvcs=false` to avoid development dependencies and local paths in build metadata. The package checksum identifies the distributed artifact.

macOS uses the Apple GPU through Metal by default. The Ubuntu and Windows packages run inference on the CPU. Each includes the same model specification and workshop configurations. Install the OS prerequisites listed in the kit README. Measure inference speed on the hardware participants will use.

Verify the actual execution path even when compatibility and settings are documented. A larger reasoning budget can allow additional checks. It can also reinforce an incorrect assumption. Assess the final feedback, allowing enough room for the model to finish within the application’s limits.

## Pinned sources

- [Qwen original model card and thinking behavior](https://huggingface.co/Qwen/Qwen3.5-4B/blob/851bf6e806efd8d0a36b00ddf55e13ccb7b8cd0a/README.md)
- [Qwen Jinja template](https://huggingface.co/Qwen/Qwen3.5-4B/blob/851bf6e806efd8d0a36b00ddf55e13ccb7b8cd0a/chat_template.jinja)
- [Unsloth model card for the selected GGUF](https://huggingface.co/unsloth/Qwen3.5-4B-GGUF/blob/e87f176479d0855a907a41277aca2f8ee7a09523/README.md)
- [llama.cpp b11233 release](https://github.com/ggml-org/llama.cpp/releases/tag/b11233)
- [llama.cpp server documentation](https://github.com/ggml-org/llama.cpp/blob/d77dd0806dc26fc418273ef99f88da11239ca41b/tools/server/README.md)
- [llama.cpp parsing of the per-request budget](https://github.com/ggml-org/llama.cpp/blob/d77dd0806dc26fc418273ef99f88da11239ca41b/tools/server/server-common.cpp#L1390)
- [llama.cpp assembly of the forced message and closing tag](https://github.com/ggml-org/llama.cpp/blob/d77dd0806dc26fc418273ef99f88da11239ca41b/tools/server/server-schema.cpp#L419)
- [llama.cpp sampler that forces the reasoning closing sequence](https://github.com/ggml-org/llama.cpp/blob/d77dd0806dc26fc418273ef99f88da11239ca41b/common/reasoning-budget.cpp)
- [llama.cpp qwen35 architecture](https://github.com/ggml-org/llama.cpp/blob/d77dd0806dc26fc418273ef99f88da11239ca41b/src/llama-arch.cpp#L41)
