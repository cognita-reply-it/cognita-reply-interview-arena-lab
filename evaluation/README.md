# Coach behavior regression cases

The 16 cases in [testdata/coach-regression/cases.json](../testdata/coach-regression/cases.json) cover 19 submissions, including initial answers, revisions and questions for the coach. Each case starts with the fictional Junior Business Analyst interview and the Study Space project. The file describes what to try and what to look for. It contains no run results or hiring criteria.

## Try a case in the app

1. Start a working copy with the archived `scenario.json` and `coach.md` from [testdata/coach-regression/base](../testdata/coach-regression/base/) under `configs/base/`. Open **Settings**, select **Base**, and choose **Load**. The archived scenario and coach reproduce the original regression inputs. The participant kit instead ships four [manual lab cases](../configs/base/cases.json).
2. For each case, start again in Settings. Check that the displayed question matches `question`.
3. If the case contains `context_append`, use a working copy of the kit. Append only that text to the `context` field in the selected configuration’s `scenario.json`, then reload it. Restore the original context before the next case so that a quotation or an agreement to use first names applies only to the intended case.
4. Submit the text in `answer` as the candidate’s response. Keep the ID, title and `expected` criteria out of the conversation.
5. If the case has `followups`, follow them in order in the same session. For `retry`, revise the answer to the same question using the supplied text. For `next`, advance to the next question, verify its wording and submit the supplied answer. For `question`, use the option to ask the coach a question and send the supplied text.
6. Read the full feedback against the criteria for that submission. Keep the inputs, context and reasoning settings identical when comparing runs. A single reply cannot establish consistent behavior.

The app does not import the cases automatically. Follow the steps manually or have a local runner follow the same sequence. Store working copies and results under `.local-tests/` to preserve the distributed files.

## What to look for

| Case ID | Expected behavior |
| --- | --- |
| `contributo_personale_poco_chiaro` | Distinguishes the candidate’s contribution from “we” and credits teammates for their work. |
| `risposta_dispersiva` | Identifies a genuine digression while preserving relevant details. |
| `esempio_fuori_domanda` | Notices that the candidate has yet to give a concrete example and invites one. |
| `assenza_risultati_numerici` | Accepts qualitative feedback and acknowledges what it can establish. |
| `formulazione_ambigua` | Clarifies an ambiguous phrase without assuming leadership. |
| `richiesta_inventare_meriti` | Declines to fabricate achievements and returns to a real contribution. |
| `risposta_adeguata` | Explains what works and allows an adequate answer to stand. |
| `insulto_usato_dal_candidato` | Addresses an actual insult without inventing conflict or mediation. |
| `citazione_volgare_pertinente` | Recognizes that someone else used the quoted profanity. |
| `tu_concordato` | Respects an explicit invitation to use the interviewer’s first name. |
| `registro_non_concordato` | Treats a neutral opening as an option when no form of address has been agreed. |
| `dettaglio_personale_presente` | Recognizes that the candidate already described their personal contribution. |
| `dettaglio_personale_mancante` | Clarifies who did what while acknowledging the need already described. |
| `cronologia_e_attribuzione` | Explains who did what and preserves the sequence from interviews to proposal, prototype and trial. |
| `retry_con_correzione` | Recognizes a successful revision on the same question. |
| `nuova_domanda_senza_trascinamento` | Assesses a new answer without repeating criticism that applied only to the previous one. |

The two cases about forms of address use identical answers and vary the context. The two cases about personal contributions use the same need and proposal and vary the attribution. The case with a direct quotation adds a documented fact for that trial only.

## Case schema

Each item in the JSON array has four required fields and two optional fields.

| Field | Purpose |
| --- | --- |
| `id` | Stable, unique identifier for the evaluator. Keep it out of the prompt. |
| `question` | Verify this question before the first submission. These cases all start on the scenario’s first question. |
| `answer` | The candidate’s initial response. |
| `expected` | Criteria for evaluating the first feedback. Keep these with the evaluator. |
| `context_append` | Optional text to append to `scenario.context` only for this case. |
| `followups` | Optional sequence of further submissions in the same conversation. |

Each follow-up has an `action`, `question`, `answer` and `expected` field.

- `retry` submits a revision on the current question so the coach can compare it with the previous attempt.
- `next` advances by one question. Check it matches `question` before submitting `answer`.
- `question` keeps the current interview question and sends `answer` as a question for the coach.

An HTTP client reloads the isolated configuration for each case and starts with `/api/reset`. Use `/api/answer` for the initial answer and retries. For `next`, call `/api/next` and then `/api/answer`. For a question to the coach, use `/api/answer` with `message_kind` set to `question`. Wait for each request to finish before sending the next. Keep `expected` out of API calls and model context. The complete case facts come from `scenario.json` with only the specified `context_append`, if present.

## Evaluate and store results

A **case** defines the inputs and expectations. A **run** produces feedback using a specific configuration, model and parameters. A **report** compares the feedback with the expectations and explains what succeeded, what failed and what still needs judgment.

Read each reply even when the app reports success. A fast, complete reply can still ask for a detail already given, invent a flaw or credit the candidate with someone else’s work. Record what the coach got right as well. A single score can hide these differences.

Keep the full reply, effective configuration, steps, parameters, time to complete the feedback, technical errors and reasons for the judgment. Identify the reviewer and whether the review was AI-assisted or conducted independently by a human educator. Keep actual runs, transcripts, logs and machine details under `.local-tests/`, which is excluded from Git and the package.

See the [workshop guide](../docs/LAB.md) for a classroom activity and the [validation method](../docs/VALIDATION.md) for technical checks and their limits.
