# Customize the interview coach and test the change

Each group uses a writable copy of the extracted kit. Set up one university project for a Junior Business Analyst interview, then compare the coach’s replies before and after a change.

## Set up the project

Open `configs/base/scenario.json` in a text editor. The single experience starts as **Progetto universitario**. Replace its title or sample facts with your project details and keep the four interview questions. In `facts`, use the three headings to distinguish what the candidate did, what the team did and which outcomes are known. Say when an outcome was not measured. Leave out identifying or confidential details.

The three Base files serve different purposes.

| File | Purpose |
| --- | --- |
| `configs/base/scenario.json` | The role, project facts and four questions. |
| `configs/base/coach.md` | The coach’s instructions. Edit **Chat mode** for the main conversation. |
| `configs/base/cases.json` | Optional manual test guide. Each of the four examples supplies a candidate answer, an expected behavior and evidence to save. Adapt the examples to the facts of your project. Keep the expectations with the group. |

## Compare four replies before and after

1. With the app open, go to **Settings → Base → Load**. Confirm that the new project facts appear and the chat starts at question 1.
2. Try one answer per question. Use the numbered question controls and record the exact answer, complete reply, and the excerpt that supports your judgment. State what you expected before looking at the reply.
3. Edit one instruction in the **Chat mode** section of `configs/base/coach.md`. For example, ask the coach to acknowledge a personal choice the candidate already explained before asking about its rationale. Keep the scenario, model and Reasoning setting fixed.
4. Use **Settings → Base → Load** again. This rereads both edited files from the extracted kit and creates a fresh session. Repeat the same four answers on the same questions, in the same order.
5. Compare the paired replies. Check whether the coach distinguishes personal and team work, asks for relevant evidence and keeps proposed future actions separate from known outcomes. Quote what changed and identify anything you would test again.

Keep actual replies and observations with your group. The exercise is ungraded.
