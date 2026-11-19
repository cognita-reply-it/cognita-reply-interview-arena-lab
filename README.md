# AI Interview Arena

AI Interview Arena is the local kit for Cognita Reply’s workshop at the Politecnico di Milano Innovator Bootcamp. Participants design a coach for a **junior job interview**. The exercise focuses on the Innovator’s role: understanding a need, deciding how a system should respond and testing whether that response helps.

> Recognise your strengths and explain them clearly. Stick to what you actually did.

The coach opens with a question about a fictional case. Based on your reply, it might ask why you made a choice, which part of the work was yours or what evidence supports a conclusion. You can ask questions or challenge the coach in return. There are no grades. The model can give poor advice, and spotting it is part of the exercise. Before using this prototype with students, review the [validation criteria](docs/VALIDATION.md) and try the [workshop with someone new to the project](docs/LAB.md).

## Workshop kit

Choose the ZIP for each participant’s operating system. In the repository, the downloads and SHA-256 files are in [packages](packages/). If you already have a ZIP, extract it into a writable folder before opening the launcher. Keep the application, runtime and configurations together.

| Platform | ZIP | Start the kit |
| --- | --- | --- |
| Apple Silicon Mac, macOS 13.3 or later | [macOS arm64](packages/AI-Interview-Arena-macOS-arm64-v0.1.8.zip) | Double-click **Start AI Interview Arena.command**. |
| Ubuntu 22.04 or 24.04 on Intel/AMD 64-bit | [Ubuntu x64](packages/AI-Interview-Arena-Ubuntu-x64-v0.1.8.zip) | Extract with `unzip` to a Linux filesystem. In a terminal in the extracted folder, run `./"Start AI Interview Arena.sh"`. |
| Windows 10 or 11 on Intel/AMD 64-bit | [Windows x64](packages/AI-Interview-Arena-Windows-x64-v0.1.8.zip) | In File Explorer choose **Extract All**, open the extracted folder, then double-click **Start AI Interview Arena.cmd**. |

**Ubuntu first launch.** The launcher checks whether the engine can load OpenMP and the C++ runtime. If a library is missing, it uses APT to install the corresponding package (`libgomp1` or `libstdc++6`). Ubuntu may ask for your sudo password. The app continues under your account. For an offline class, install the missing packages on each machine before using `--offline`. If the launcher itself reports “Permission denied”, extract the ZIP with `unzip` into your home folder to restore its permissions and symlinks.

**Windows first launch.** If the engine needs the Microsoft Visual C++ v14 x64 Redistributable, the launcher downloads the installer from Microsoft, verifies its signature and asks Windows to run it. Approve the administrator prompt to continue. For offline setup, place the official `VC_redist.x64.exe` beside the launcher before starting. On managed computers, IT can install the prerequisite in advance. [Microsoft’s supported downloads](https://learn.microsoft.com/en-us/cpp/windows/latest-supported-vc-redist/)

## First run and workshop

1. Leave the launcher window open. On first use, the app downloads **2.74 GB** from Hugging Face to the kit’s `models/` folder, verifies its SHA-256 checksum and loads it. Allow about **4 GB of free space**. An interrupted download can resume. With prerequisites installed and verified weights present, the app runs offline.
2. Open **Guide** and check the version on the instructor’s computer and each group’s computer. All three platforms show **Kit v0.1.8**. Every ZIP includes the same slide deck and workshop. Open **Put the coach to the test** in the guide to compare replies and exchange cases. [Workshop instructions](docs/LAB.md)
3. In the browser, choose a practice case and question, read the facts and reply in the chat. Answer a follow-up, add a detail or ask the coach why it challenged your conclusion. Use the numbered buttons to return to earlier questions. **Settings → Start over** clears the session.
4. To finish, choose **Settings → Stop the app** or press **Ctrl+C** in the launcher window. Closing the browser tab alone leaves the engine running.

Each ZIP includes the app and inference engine. Participants need a browser and the platform prerequisites above. The kit requires no Python, Node, Docker, Go, compiler, AI account or paid inference service. macOS uses Metal by default. Ubuntu and Windows run the model on the CPU. Response time depends on the processor and available memory.

### If macOS blocks the app

The prototype has no Developer ID signature or notarization. Gatekeeper may ask for approval when you download it from the internet. Verify the package source and checksum, try opening the app, then go to **System Settings → Privacy & Security → Open Anyway** if macOS offers that option for the file. You may also need to approve the bundled engine. On a managed Mac, contact IT if the policy prevents you from opening it.

Keep Gatekeeper, SIP and quarantine protections enabled. Check the downloaded package on a fresh Mac before running the class. A build tested on the developer’s computer does not establish how Gatekeeper will handle a new download.

### Use an existing model or run offline

The expected file is `Qwen3.5-4B-Q4_K_M.gguf`. Its checksum is pinned in the [manifest](manifest.json). Copy it to `models/` next to the app. Create the folder if needed. At each launch, the app checks the size and checksum and reuses a valid file.

From the extracted kit folder on macOS or Ubuntu, you can also point to a model elsewhere or start offline with the model in place. The first path below is an example Mac volume path. On Ubuntu, substitute the path to your GGUF.

```sh
./ai-interview-arena --model "/Volumes/USB/Qwen3.5-4B-Q4_K_M.gguf"
./ai-interview-arena --offline
```

In Windows PowerShell, the equivalent commands are:

```powershell
.\ai-interview-arena.exe --model "D:\Models\Qwen3.5-4B-Q4_K_M.gguf"
.\ai-interview-arena.exe --offline
```

The `--model` option verifies the file and uses it in place. Its contents must match the pinned model, regardless of the filename. To use a fixed external path from Finder, edit the invocation in the `.command` file. For the default setup, copy the model to `models/`.

If you interrupt a download, the partial file stays in the kit. On the next launch the app resumes with HTTP Range. It also retries after network failures and explains how to continue if the retry fails. The app rejects a file with the wrong checksum and keeps it for inspection. Move it elsewhere or to the Trash before retrying.

## Customize the coach

The **Base** lab files are grouped under `configs/base/`:

- `configs/base/coach.md` contains the coach’s instructions and feedback criteria.
- `configs/base/scenario.json` starts with one **Progetto universitario** experience for the Junior Business Analyst interview. Replace the example facts with your project. Describe the candidate’s contribution, the team’s work and the known outcomes separately. Keep the four questions.
- `configs/base/cases.json` is an optional guide with one manual test per question. Adapt the answers to your facts before testing.

Edit the files in a text editor. In **Settings**, select **Base** or **Demo** and click **Load**. The app rereads the files and starts a fresh conversation with the model already in memory. Save any feedback you want to compare before loading. Open **Configuration files** to inspect the loaded version. If the JSON is invalid, fix the file and click **Load** again.

The multi-experience format uses an `experiences` array. Each entry has a unique, stable `id`, a `title`, a `facts` string and a list of `questions`. You can organize the facts with headings on their own lines, such as `## Your contribution`, or use plain text. To add an experience, copy an entry and edit those four fields. The earlier format with top-level `experience` and `questions` is also supported. Use one format per file. Each configuration describes one fictional person. Load another scenario to switch people and start a new session.

Load **Base** after editing the scenario and run the four tests. Edit one instruction in the **Chat mode** section of `coach.md`, load **Base** again and repeat the same four inputs with the same settings. Save both sets of replies before loading again.

The [manual test cases](configs/base/cases.json) give sample inputs, expected behavior and evidence to record. Toggle **Reasoning** to compare the same model with reasoning on and off. The engine uses the template embedded in the GGUF, with up to **512 reasoning tokens** and **900 tokens in total** by default. The complete-reply timeout is **60 seconds on macOS** and **5 minutes in the Ubuntu and Windows CPU kits**. CPU inference may take longer on a first turn. Launch with `--reply-timeout 60s` to enforce the classroom timing target. A truncated reply or one without final feedback is reported as a failure and excluded from the conversation.

## What stays on your computer

The app and engine listen only on `127.0.0.1`. The app generates a random key for its connection to the engine at startup. The interface loads its fonts, code and other assets locally. The app has no database, accounts or telemetry. Messages and feedback stay in session memory. Local testing tools can deliberately save inputs, outputs and metadata in private folders excluded from Git and the kit.

A session holds up to 200 exchanges. Each question in each experience has its own chat. For its next reply, the model receives the case facts, the question and up to ten recent exchanges from that conversation, as space permits. Return to a question to read its full history.

Stopping the app, starting over or loading a configuration clears session memory. Refreshing the browser preserves it. The app counts tokens using the engine’s template and can leave older exchanges out of the model’s prompt. If the active text and case facts still exceed the limit, it reports an error. Only one generation runs at a time. Two tabs share the same session.

## Troubleshooting

| What you see | What to do |
| --- | --- |
| Download interrupted | Restart with internet access. Leave the `.part` file in place with its original extension. |
| Wrong size or checksum | Move the file elsewhere or to the Trash, then restart. Alternatively, point `--model` at the correct file. |
| Engine missing | Use the complete ZIP. Keep the executable inside the extracted kit. |
| Kit already running | Use the URL in the first Terminal window, or stop that instance before opening another. The OS releases the lock on exit. |
| Browser did not open | Copy the URL from Terminal. You can also start with `--no-browser` and open the URL yourself. |
| Feedback truncated or timed out | Record a failed attempt. Try a shorter answer, turn reasoning off or check memory use by other apps. |
| Invalid configuration | Read the reported error, fix `scenario.json` and reload. |
| Forced shutdown with SIGKILL | If the kit’s `llama-server` process survives, quit that specific process in your system’s process manager. |

Operational logs are in the kit’s `.arena/` folder. They contain engine information and timings. The app does not write chat transcripts there. Use the Trash for any files you decide to remove.

To test CPU-only inference, run:

```sh
./ai-interview-arena --offline --cpu
```

On Windows, use `.\ai-interview-arena.exe --offline --cpu` from PowerShell.

## Source, build and verification

These instructions apply to the source checkout. The extracted participant kit contains neither Go source nor build scripts. The app uses the Go standard library and embeds HTML, CSS and JavaScript. Development dependencies are downloaded inside the checkout. The manifest pins the Go toolchain, llama.cpp and model weights. It uses fixed URLs and keeps weights outside Git LFS and GitHub mirrors.

The React frontend and Stageshow components live in [frontend/src](frontend/src/). After a UI change, use Node 20 or later to run `npm ci`, `npm run typecheck` and `npm run build` in `frontend/` before rebuilding Go. This produces `web/index.html` with scripts, styles and licenses embedded. The distributed app needs no Node runtime or external assets. [Frontend build instructions](frontend/README.md)

```sh
./scripts/build-macos.sh --build-only
./scripts/build-macos.sh
```

The first command prints the path to a new binary under `.cache/build-macos-…/AI-Interview-Arena-macOS-arm64-v0.1.8/`. The second builds the full kit, keeps the staging folder and creates a ZIP under `packages/`. The package includes the runtime, configurations, licenses, documentation and teaching cases. The build excludes weights, test tooling, test results, operational logs and information about the development machine. It preserves existing packages. Add `--offline` to use cached development archives and prohibit downloads.

Use `./scripts/build-macos.sh --stage-only` to assemble and run the complete kit before creating a ZIP. It applies the same file allowlist and prints the staging path.

To build the Ubuntu and Windows packages on the same Apple Silicon development machine, use Python 3 and the cross-build script. Its platform-specific archives are pinned in `scripts/desktop-runtimes.json`. Each generated kit records the selected runtime in its own `manifest.json`. Use `--offline` when the pinned archives are already cached:

```sh
./scripts/build-kit.sh linux-x64 --offline
./scripts/build-kit.sh windows-x64 --offline
```

Run the commands from the source checkout on Apple Silicon macOS. For a build without cached archives, run `./scripts/build-kit.sh windows-x64`. The Windows ZIP under `packages/` contains the x64 executable, the CPU inference runtime and the Base lab files. On Windows, use **Extract All** and double-click **Start AI Interview Arena.cmd** in the extracted folder. From PowerShell in that folder, use `& ".\Start AI Interview Arena.cmd"`.

To run the source tests with the development toolchain:

```sh
GOROOT="$PWD/.tools/go" GOCACHE="$PWD/.cache/go-build" .tools/go/bin/go test ./...
```

**Participant requirements.** A computer matching the selected kit, its system runtime and a browser. The ZIP ships the compiled Go app, `llama-server` and its native libraries. The launcher uses the system shell. JavaScript runs in the browser. The engine runs as a second local process that the app starts and stops.

**Development requirements.** Go recompiles the source. Testers may use Python or browser automation separately. The public Go tests cover app behavior, including configuration reload. The source checkout also retains the [Study Space regression fixtures](evaluation/README.md). Local test harnesses, reports and machine metadata stay private.

- [Manual lab cases](configs/base/cases.json)
- [Guided workshop](docs/LAB.md)
- [Validation method and acceptance criteria](docs/VALIDATION.md)
- [Model provenance, licenses and parameters](docs/MODEL.md)

## Privacy before publication

The repository rules are in `AGENTS.md`. Publish only fictional cases and portable instructions. Keep harnesses, results, logs, screenshots and machine details in the ignored local folders. Git still tracks files committed before a `.gitignore` entry was added. Review outgoing history and ZIP contents before each authorized push.

The publication check is `scripts/check-publication.py`. The hook in `.githooks/pre-push` prefers a copy installed in Git’s hooks directory outside the checkout so that switching to an older branch preserves the check. Preserve existing hooks before installation. To check a commit range manually, run `python3 scripts/check-publication.py --base origin/main --head HEAD`. These are development tools. Keep the privacy check enabled for pushes.

## Licenses

The original code and configurations retain the repository’s **MIT license**. Qwen3.5-4B and the Unsloth quantization declare Apache 2.0. llama.cpp uses MIT. Go and the vendored dependencies retain their own notices. The quantizer names the base model but does not specify the exact conversion input commit. See the [third-party notices](third_party/) and [project license](LICENSE).
