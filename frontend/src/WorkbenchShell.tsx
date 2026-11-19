import {useState, type ReactNode} from 'react';
import './workbench.css';

export interface WorkbenchShellProps {
  header: ReactNode;
  session: ReactNode;
  dossier: ReactNode;
  conversation: ReactNode;
}

/** Keep the case facts beside the conversation, or in a disclosure on smaller screens. */
export function WorkbenchShell({header, session, dossier, conversation}: WorkbenchShellProps) {
  const [factsOpen, setFactsOpen] = useState(false);
  return (
    <div className="arena-app">
      <a className="skip-link" href="#answer">Skip to the message field</a>
      <header className="arena-header">{header}</header>
      <main className="arena-main">
        <section className="arena-session" aria-label="Practice case and questions">{session}</section>
        <div className="arena-workspace">
          <aside className="arena-dossier" aria-label="Case facts">
            <button type="button" className="facts-toggle" aria-expanded={factsOpen} aria-controls="case-dossier" onClick={() => setFactsOpen(open => !open)}>
              Case facts <span aria-hidden="true">{factsOpen ? '−' : '+'}</span>
            </button>
            <div id="case-dossier" className="dossier-content" data-expanded={factsOpen}>{dossier}</div>
          </aside>
          <section className="arena-conversation" aria-labelledby="chat-heading">{conversation}</section>
        </div>
      </main>
    </div>
  );
}

export default WorkbenchShell;
