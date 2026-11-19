interface CaseFactsProps {
  facts: string;
  experienceId: string;
}

export function CaseFacts({facts, experienceId}: CaseFactsProps) {
  // Plain text stays intact. Only an explicit second-level heading creates a section.
  const blocks = facts.split(/^##[ \t]+([^\r\n]+)\r?$/m);
  if (blocks.length === 1) return <>{facts}</>;

  const sections = [];
  for (let index = 1; index < blocks.length; index += 2) {
    sections.push(<section className="fact-section" key={experienceId + ':' + index}>
      <h3>{blocks[index]}</h3>
      <p>{blocks[index + 1].trim()}</p>
    </section>);
  }
  return <>{blocks[0].trim() && <p>{blocks[0].trim()}</p>}{sections}</>;
}

export function GuideContent({onOpenSettings}: {onOpenSettings: () => void}) {
  const kitVersion = document.querySelector<HTMLMetaElement>('meta[name="arena-kit-version"]')?.content;
  return <>
    <h3 className="guide-heading">Prepare for the interview</h3>
    <ol className="guide-steps">
      <li>Choose a case, read the facts, and answer the opening question in the chat.</li>
      <li>Keep the conversation going. If you write “we decided,” the coach may ask who proposed the decision or what observation supported it.</li>
      <li>You can ask for clarification, correct the coach, or say when a detail is missing. If you switch cases or questions, you can come back and reread the conversation.</li>
    </ol>
    <details className="guide-workshop">
      <summary>Put the coach to the test</summary>
      <ol>
        <li><strong>Set up your project.</strong> Edit <code>configs/base/scenario.json</code> in the extracted kit. Describe the candidate’s contribution, the team’s work and the known outcomes separately. Keep the four questions.</li>
        <li><strong>Try the initial coach.</strong> In Settings, select Base and choose <strong>Load</strong>. Try one answer per question. The optional <code>configs/base/cases.json</code> file gives example inputs, expected behavior and evidence to record.</li>
        <li><strong>Change one instruction.</strong> Edit the Chat mode section of <code>configs/base/coach.md</code>. Select Base and choose <strong>Load</strong> again to reread both files and start a fresh session.</li>
        <li><strong>Repeat the four tests.</strong> Keep the project facts, answers and Reasoning setting fixed. Compare the replies and save a quote that supports each judgment.</li>
      </ol>
      <button type="button" className="native-button secondary" onClick={onOpenSettings}>Open Settings</button>
    </details>
    <p className="privacy-note">Leave out identifying or confidential details. This exercise is ungraded. The conversation stays on your computer and is cleared when you start over, load a configuration, or stop the app.</p>
    {kitVersion && <small className="guide-version">Kit v{kitVersion}</small>}
  </>;
}
