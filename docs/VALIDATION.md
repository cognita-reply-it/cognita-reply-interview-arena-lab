# How to validate the kit

This public guide describes the validation method. Keep the report for each run locally. Record what you actually tested, what happened and what the evidence covers. Keep machine-specific details and measurements in that report.

## Two questions to answer

1. **Does the kit work?** Can it start, verify the download, return complete feedback, accept another attempt and shut down cleanly?
2. **Does the coach give sound advice?** Does it understand the answer, stay on topic, respect the facts and recognize an answer that already works?

Judge the content of each reply separately from technical completion. A short response time or a large reasoning budget says little on its own about the usefulness of the advice.

## Runtime checks

| Check | Starting condition | What to demonstrate |
| --- | --- | --- |
| Prerequisite setup | A fresh Ubuntu or Windows system with a missing engine library. | Open the launcher, approve the system installer and verify the engine starts. Relaunch and check that it skips installation. |
| Offline and declined setup | A missing prerequisite and an offline launch, or a user who declines administrator access. | The launcher reports what is needed before the model download. On Windows, also try an official installer beside the launcher and verify that an invalid installer is rejected. |
| First launch | Fresh kit folder containing neither the model nor a partial download. | The kit downloads the entire model from the pinned source, verifies its SHA-256 hash, loads it and returns complete feedback. |
| Reuse | The kit already contains a valid model file. | The kit verifies the existing file and completes another exchange without downloading it again. |
| Interrupted download | A partial download is available. | The transfer resumes without unnecessarily starting from zero, and the completed file passes the integrity check. |
| Local file | A valid model is in another folder. | `--model` uses the verified file in place. |
| Offline use | External access is blocked and localhost remains available. | The interface, engine and feedback work without external requests. |
| Conversation | A general answer followed by a personal detail on the same question. | The coach asks a specific follow-up and uses the detail in the next reply. After a page refresh, each message appears once, in order. |
| Case switching | Two experiences with different people and outcomes. | The second chat opens on its own question. Switching back restores the original messages. |
| Customization | A running kit extracted from the ZIP and a conversation in progress. | Edit both files under `configs/base/` and use **Settings → Base → Load**. Check the new facts, an empty conversation and the new coach instruction in the next reply or engine request. An invalid edit must preserve the last valid configuration. |
| Shutdown | The kit’s app and engine are running. | The processes started for the test terminate and unrelated services keep running. |

Measure package transfer (if actually downloaded), extraction, model download, hash verification, loading and the first exchange separately. A fresh folder can still benefit from memory and CDN caches. Record which caches were reused. Testing in a fresh folder does not establish behavior on a newly installed operating system.

The classroom targets are **30–45 seconds for complete feedback**, with **60 seconds as the acceptance limit**, and **15 minutes for installation and first use**. Verify these targets on the setup under test. A CPU reply completed after 60 seconds can confirm that inference works, but it misses the classroom timing target. Use `--reply-timeout 60s` to enforce that target on CPU kits. Time the response through its final token. A reply that is empty, times out or ends at the token limit fails the completion check.

## Behavior checks

Use the [four manual cases](../configs/base/cases.json) for the classroom comparison. The broader regression cases in `testdata/coach-regression/` use the preserved Study Space inputs in the source checkout. Add cases for each behavior you intend to change. Use identical inputs and conditions, repeat the cases and retain failures. Vary one factor at a time, such as the instructions or the reasoning budget.

Include an adequate answer, a retry that corrects a problem, and paired cases that differ in a meaningful way. Compare a personal insult with an attributed quotation, an agreed form of address with an unspecified one, and an explicit personal contribution with an ambiguous one. Count unjustified corrections as errors even when the same advice might apply elsewhere.

Assess complete sessions with at least two experiences, different questions, clarifications and a return to an earlier experience. Look for continuity across turns. One sound suggestion is insufficient evidence of continuity, and one minor stylistic lapse need not make a whole session unusable.

Use these three session outcomes:

- **Independent use.** The coach offers specific help grounded in the facts, recognizes progress and keeps the experiences distinct. The facilitator never needs to correct it.
- **Light supervision.** The session remains useful, but the coach occasionally needs correction or asks for information the candidate has already given. A serious error fits this category only if the coach explicitly withdraws it, corrects it and does not rely on it again.
- **Unusable.** A serious error stands, substantial corrections are repeatedly needed, or the coach offers too little help even without an obvious factual error.

Serious errors include inventing achievements or outcomes, mixing up experiences, persisting with a false criticism and helping the candidate give a misleading impression. Record recoveries as well as the original errors. Assess unfounded approval separately from genuine problems the coach missed.

Before testing, define the practical acceptance threshold, facts for each profile and rules for how the simulated candidate will respond. Keep development sessions separate from new verification conversations. Freeze the coach before verification and avoid tuning it on the verification cases. Report counts by session and profile, repeated runs, doubtful cases and facilitator interventions. Automated judgments are provisional. Include a short trial with someone unfamiliar with the project.

For each case, the report should record:

- The purpose in plain language and the context needed to understand it.
- The interview question, the submitted answer and the expected behavior.
- The complete feedback actually received.
- Whether the reply was complete, how long it took and any technical error.
- The qualitative judgment, supporting evidence and whether the reviewer was human or automated.
- Anything unverified or uncertain.

Mark each outcome **passed**, **failed** or **awaiting assessment**. A successful HTTP response establishes only that the request succeeded.

## Scope and privacy

The local test suite and its results are excluded from Git. Distribute only the application, engine, configurations, teaching cases, documentation and licenses. Keep machine reports, development screenshots, transcripts and logs private. See the publication instructions in [AGENTS.md](../AGENTS.md).

Use the package matching the target machine. On Ubuntu, verify the extracted executable permissions, library symlinks and OpenMP dependency. On Windows, check the x64 Visual C++ runtime, run the extracted CMD launcher, complete a chat reply and stop the app. Distinguish an emulated test from one on a native Windows desktop. On a Mac, check Gatekeeper on a fresh download. Repeat on representative classroom machines and keep system protections enabled.
