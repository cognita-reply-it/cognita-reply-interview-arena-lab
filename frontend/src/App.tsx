import {useEffect, useId, useRef, useState, type FormEvent, type ReactNode} from 'react';
import {Icon} from '../vendor/stageshow/kit/parts/Icon';
import {Spinner} from '../vendor/stageshow/kit/parts/Spinner';
import {Conversation} from './Conversation';
import {WorkbenchShell} from './WorkbenchShell';
import {CaseFacts, GuideContent} from './WorkshopContent';
import type {AnswerContext, AnswerRequest, AnswerResult, InterviewState} from './types';
import './app.css';

class ArenaAPIError extends Error {
  constructor(message: string, readonly code?: string, readonly sessionId?: string) {super(message);}
}

async function api<T>(path: string, payload?: object): Promise<T> {
  let response: Response;
  try {
    response = await fetch(path, {
      method: payload === undefined ? 'GET' : 'POST',
      headers: payload === undefined ? {} : {'Content-Type': 'application/json'},
      body: payload === undefined ? undefined : JSON.stringify(payload),
      cache: 'no-store',
    });
  } catch {
    throw new ArenaAPIError('Cannot connect to the local app. Check that the launcher window is still open.', 'unreachable');
  }
  let data;
  try { data = await response.json(); }
  catch { throw new ArenaAPIError('Could not read the response from the app.', 'unreadable'); }
  if (!response.ok) throw new ArenaAPIError(data.error || 'The operation failed.', data.code, data.session_id);
  return data as T;
}

const message = (error: unknown) => error instanceof Error ? error.message : 'The operation failed.';
const errorCode = (error: unknown) => error instanceof ArenaAPIError ? error.code : undefined;
const contextKey = (context: AnswerContext) => [context.session_id, context.experience_id, context.question_index].join(':');

interface FocusOrigin {
  element: Element;
  interaction: number;
}

interface PendingSend {
  request: AnswerRequest;
  originalDraft: string;
  focus: FocusOrigin | null;
  deliveryUnknown: boolean;
}

type SessionEvent = {type: 'reset' | 'stopped'; sessionId: string};
const sessionEventKey = (type: SessionEvent['type']) => 'interview-arena:' + type;

function readSessionEvent(type: SessionEvent['type']): SessionEvent | null {
  try {
    const sessionId = localStorage.getItem(sessionEventKey(type));
    return sessionId ? {type, sessionId} : null;
  } catch {return null;}
}

function Modal({open, title, onClose, children}: {open: boolean; title: string; onClose: () => void; children: ReactNode}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const titleId = useId();
  useEffect(() => {
    if (open && !dialog.current?.open) dialog.current?.showModal();
    else if (!open && dialog.current?.open) dialog.current?.close();
  }, [open]);
  return <dialog ref={dialog} className="dialog" aria-labelledby={titleId} onCancel={onClose} onClose={onClose} onClick={event => {if (event.target === event.currentTarget) onClose();}}>
    <div className="dialog-header"><h2 id={titleId}>{title}</h2><button className="native-button ghost small" type="button" aria-label="Close dialog" onClick={onClose}><Icon name="close" size={20}/></button></div>
    <div className="dialog-body">{children}</div>
  </dialog>;
}

export default function App() {
  const [state, setState] = useState<InterviewState | null>(null);
  const stateRef = useRef<InterviewState | null>(null);
  const drafts = useRef(new Map<string, string>());
  const pending = useRef(new Map<string, PendingSend>());
  const recovering = useRef<string | null>(null);
  const epoch = useRef(0);
  const action = useRef(false);
  const syncing = useRef<number | null>(null);
  const stoppedRef = useRef(false);
  const channel = useRef<BroadcastChannel | null>(null);
  const interaction = useRef(0);
  const [draft, setDraft] = useState('');
  const [sending, setSending] = useState(false);
  const [changing, setChanging] = useState(false);
  const [stopped, setStopped] = useState(false);
  const [unavailable, setUnavailable] = useState(false);
  const [thinking, setThinking] = useState(true);
  const [configChoice, setConfigChoice] = useState<string | null>(null);
  const [error, setError] = useState('');
  const [feedbackError, setFeedbackError] = useState('');
  const [notice, setNotice] = useState('');
  const [elapsed, setElapsed] = useState(0);
  const [modal, setModal] = useState<'guide' | 'lab' | null>(null);
  const [moreFacts, setMoreFacts] = useState(false);
  const facts = useRef<HTMLDivElement>(null);
  const busy = sending || Boolean(state?.busy);
  const locked = busy || changing || stopped || !state;

  function clearPrivateState() {
    drafts.current.clear();
    pending.current.clear();
    recovering.current = null;
    setDraft('');
    setConfigChoice(null);
    setError('');
    setFeedbackError('');
    setNotice('');
  }

  function clearWorkspace(terminal: boolean) {
    epoch.current++;
    action.current = false;
    syncing.current = null;
    stoppedRef.current = terminal;
    stateRef.current = null;
    clearPrivateState();
    setState(null);
    setSending(false);
    setChanging(false);
    setModal(null);
    setStopped(terminal);
    setUnavailable(false);
  }

  function notifyTabs(type: SessionEvent['type'], sessionId: string) {
    const event: SessionEvent = {type, sessionId};
    // Persist only the identifier and event type so a frozen tab can catch up on resume.
    try {localStorage.setItem(sessionEventKey(type), sessionId);} catch { /* Storage can be unavailable. */ }
    channel.current?.postMessage(event);
  }

  function markStopped(sessionId = stateRef.current?.session_id) {
    clearWorkspace(true);
    if (sessionId) notifyTabs('stopped', sessionId);
  }

  function handleStopped(cause: unknown) {
    if (!(cause instanceof ArenaAPIError) || cause.code !== 'session_stopped') return false;
    markStopped(cause.sessionId);
    return true;
  }

  function captureFocus(): FocusOrigin | null {
    const element = document.activeElement;
    return element && element !== document.body ? {element, interaction: interaction.current} : null;
  }

  function focusAfterUpdate(id: string, key: string, origin: FocusOrigin | null) {
    if (!origin) return;
    const atEpoch = epoch.current;
    requestAnimationFrame(() => {
      const current = stateRef.current;
      if (epoch.current !== atEpoch || !current || contextKey(current) !== key) return;
      const target = document.getElementById(id);
      const active = document.activeElement;
      if (target && interaction.current === origin.interaction && document.hasFocus()
          && (active === origin.element || active === document.body)) {
        target.focus();
      }
    });
  }

  function accept(next: InterviewState, followSelection = true) {
    if (readSessionEvent('stopped')?.sessionId === next.session_id) {
      clearWorkspace(true);
      return;
    }
    if (readSessionEvent('reset')?.sessionId === next.session_id) {
      clearWorkspace(false);
      void sync();
      return;
    }
    const previous = stateRef.current;
    const key = contextKey(next);
    const sameSession = previous?.session_id === next.session_id;
    const sameSelection = previous && contextKey(previous) === key;
    setUnavailable(false);
    if (sameSession && !sameSelection && !followSelection) return;
    if (previous && !sameSession) clearPrivateState();

    const received = new Set(next.turns.map(turn => turn.request_id));
    let completed: PendingSend | undefined;
    for (const [pendingKey, item] of pending.current) {
      if (!received.has(item.request.request_id)) continue;
      pending.current.delete(pendingKey);
      if (recovering.current === item.request.request_id) recovering.current = null;
      if (drafts.current.get(pendingKey) === item.originalDraft) drafts.current.delete(pendingKey);
      completed = item;
    }

    stateRef.current = next;
    setState(next);
    if (!sameSelection || completed) setDraft(drafts.current.get(key) ?? '');
    if (completed) {
      setError('');
      setFeedbackError('');
      if (contextKey(completed.request.expected_context) === key) {
        focusAfterUpdate('answer', key, completed.focus);
      } else setNotice('Your message was saved under the previous question.');
    }
    if (!next.busy && recovering.current) {
      recovering.current = null;
      setFeedbackError('The request did not complete. You can send it again.');
    }
  }

  async function sync(followSelection = false) {
    if (action.current || stoppedRef.current || syncing.current !== null) return;
    const token = epoch.current;
    syncing.current = token;
    try {
      const next = await api<InterviewState>('/api/state');
      if (epoch.current === token) accept(next, followSelection);
    } catch (cause) {
      if (epoch.current !== token) return;
      if (handleStopped(cause)) return;
      if (!stateRef.current && readSessionEvent('stopped') && errorCode(cause) === 'unreachable') clearWorkspace(true);
      else if (errorCode(cause) === 'unreachable') setUnavailable(true);
      else if (!stateRef.current) setError(message(cause));
    } finally {
      if (syncing.current === token) syncing.current = null;
    }
  }

  useEffect(() => {
    try {channel.current = new BroadcastChannel('interview-arena');} catch { /* Storage events also carry session changes. */ }
    const updateInteraction = () => {interaction.current++;};
    function receive(event: SessionEvent | null) {
      if (!event || stateRef.current?.session_id !== event.sessionId) return;
      clearWorkspace(event.type === 'stopped');
      if (event.type === 'reset') void sync();
    }
    function resume() {
      receive(readSessionEvent('stopped'));
      receive(readSessionEvent('reset'));
      void sync();
    }
    const onStorage = (event: StorageEvent) => {
      if (event.key === sessionEventKey('stopped')) receive(readSessionEvent('stopped'));
      if (event.key === sessionEventKey('reset')) receive(readSessionEvent('reset'));
    };
    const onBroadcast = (event: MessageEvent<SessionEvent>) => receive(event.data);
    const onVisibility = () => {if (!document.hidden) resume();};
    channel.current?.addEventListener('message', onBroadcast);
    window.addEventListener('storage', onStorage);
    window.addEventListener('focus', resume);
    window.addEventListener('pageshow', resume);
    document.addEventListener('visibilitychange', onVisibility);
    document.addEventListener('focusin', updateInteraction);
    document.addEventListener('keydown', updateInteraction, true);
    document.addEventListener('pointerdown', updateInteraction, true);
    resume();
    return () => {
      epoch.current++;
      syncing.current = null;
      channel.current?.close();
      channel.current = null;
      window.removeEventListener('storage', onStorage);
      window.removeEventListener('focus', resume);
      window.removeEventListener('pageshow', resume);
      document.removeEventListener('visibilitychange', onVisibility);
      document.removeEventListener('focusin', updateInteraction);
      document.removeEventListener('keydown', updateInteraction, true);
      document.removeEventListener('pointerdown', updateInteraction, true);
    };
  }, []);

  useEffect(() => {
    if (!busy) return;
    const started = performance.now();
    setElapsed(0);
    const timer = window.setInterval(() => setElapsed((performance.now() - started) / 1000), 250);
    return () => clearInterval(timer);
  }, [busy]);

  useEffect(() => {
    const scroll = facts.current;
    if (!scroll) return;
    const measure = () => setMoreFacts(scroll.scrollHeight - scroll.clientHeight - scroll.scrollTop > 2);
    const observer = new ResizeObserver(measure);
    observer.observe(scroll);
    if (scroll.firstElementChild) observer.observe(scroll.firstElementChild);
    scroll.addEventListener('scroll', measure, {passive: true});
    measure();
    return () => {observer.disconnect(); scroll.removeEventListener('scroll', measure);};
  }, [state?.session_id, state?.experience_id, state?.scenario.experience]);

  useEffect(() => {
    if (!state?.busy || sending || stopped) return;
    const timer = window.setInterval(() => void sync(true), 1500);
    return () => clearInterval(timer);
  }, [state?.busy, sending, stopped]);

  function edit(value: string) {
    setDraft(value);
    if (state) drafts.current.set(contextKey(state), value);
    setError('');
    setFeedbackError('');
  }

  function startAction() {
    action.current = true;
    syncing.current = null;
    setError('');
    setFeedbackError('');
    setNotice('');
    return ++epoch.current;
  }

  async function recoverState(token: number) {
    try {
      const next = await api<InterviewState>('/api/state');
      return epoch.current === token ? next : null;
    } catch (cause) {
      if (epoch.current === token) handleStopped(cause);
      return null;
    }
  }

  async function change(path: string, payload: object, moveFocus = false) {
    if (locked || !state || action.current) return false;
    const origin = moveFocus ? captureFocus() : null;
    const token = startAction();
    setChanging(true);
    try {
      const next = await api<InterviewState>(path, payload);
      if (epoch.current !== token) return false;
      accept(next);
      if (epoch.current !== token) return false;
      if (next.session_id !== state.session_id) notifyTabs('reset', state.session_id);
      if (moveFocus) focusAfterUpdate('chat-heading', contextKey(next), origin);
      return true;
    }
    catch (cause) {
      if (epoch.current !== token) return false;
      if (handleStopped(cause)) return false;
      const next = await recoverState(token);
      if (epoch.current !== token) return false;
      if (next) {
        accept(next);
        if (epoch.current !== token) return false;
        if (next.session_id !== state.session_id) {
          notifyTabs('reset', state.session_id);
          if (path === '/api/reset' || (path === '/api/config' && 'name' in payload && next.config === payload.name)) return true;
        }
      }
      setError(message(cause));
      return false;
    } finally {
      if (epoch.current === token) {
        action.current = false;
        setChanging(false);
      }
    }
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (locked || !state || state.error || action.current) return;
    const answer = draft.trim();
    if (!answer) return;
    const {session_id, experience_id, question_index} = state;
    const expected_context = {session_id, experience_id, question_index};
    const key = contextKey(expected_context);
    const previous = pending.current.get(key);
    const request: AnswerRequest = previous?.request.answer === answer && (previous.request.thinking === thinking || previous.deliveryUnknown)
      ? previous.request
      : {answer, thinking, message_kind: 'chat', request_id: crypto.randomUUID(), expected_context};
    const origin = captureFocus();
    const focus = origin && event.currentTarget.contains(origin.element) ? origin : null;
    const item = {request, originalDraft: draft, focus, deliveryUnknown: previous?.request === request && previous.deliveryUnknown};
    pending.current.set(key, item);
    const token = startAction();
    setSending(true);
    try {
      const result = await api<AnswerResult>('/api/answer', request);
      if (epoch.current === token) accept(result.state);
    } catch (cause) {
      if (epoch.current !== token) return;
      if (handleStopped(cause)) return;
      item.deliveryUnknown ||= errorCode(cause) === 'unreachable' || errorCode(cause) === 'unreadable';
      const next = await recoverState(token);
      if (epoch.current !== token) return;
      const committed = next?.turns.some(turn => turn.request_id === request.request_id);
      const staleSelection = next && contextKey(next) !== key;
      const waiting = !committed && !staleSelection && next?.busy && errorCode(cause) !== 'context_changed';
      if (waiting) recovering.current = request.request_id;
      if (next) accept(next);
      if (epoch.current !== token || committed || waiting) return;
      if (next && next.session_id !== session_id) {
        setError('The session changed. Drafts from the previous session were cleared.');
      } else if (staleSelection || errorCode(cause) === 'context_changed') {
        setError(staleSelection
          ? 'The question changed in another tab. Go back to the previous question to find your draft.'
          : message(cause));
      } else {
        setFeedbackError(message(cause));
        focusAfterUpdate('answer', key, focus);
      }
    } finally {
      if (epoch.current === token) {
        action.current = false;
        setSending(false);
      }
    }
  }

  async function shutdown() {
    if (locked || !state || action.current) return;
    const token = startAction();
    setChanging(true);
    try {
      const result = await api<{session_id?: string}>('/api/shutdown', {});
      if (epoch.current === token) markStopped(result.session_id ?? state.session_id);
    } catch (cause) {
      if (epoch.current !== token) return;
      if (handleStopped(cause)) return;
      try {
        const next = await api<InterviewState>('/api/state');
        if (epoch.current === token) accept(next);
      } catch (probeError) {
        if (epoch.current !== token) return;
        if (handleStopped(probeError)) return;
        if (errorCode(probeError) === 'unreachable') {
          clearWorkspace(false);
          setUnavailable(true);
          return;
        }
      }
      if (epoch.current === token) setError(message(cause));
    } finally {
      if (epoch.current === token) {
        action.current = false;
        setChanging(false);
      }
    }
  }

  if (stopped) return <main className="arena-loading"><div className="brand-name">Cognita Reply</div><h1>Interview Arena</h1><p role="status">The app has stopped. You can close this tab.</p></main>;
  if (unavailable) return <main className="arena-loading"><div className="brand-name">Cognita Reply</div><h1>Interview Arena</h1><p role="status">Cannot connect to the local app.</p><button className="native-button secondary" type="button" onClick={() => void sync()}>Retry</button></main>;
  if (!state) return <div className="arena-loading" role="status"><div className="brand-name">Cognita Reply</div><h1>Interview Arena</h1>{error ? <p role="alert">{error}</p> : <p><Spinner size={18}/> Loading…</p>}<button className="native-button secondary" type="button" onClick={() => location.reload()}>Reload</button></div>;

  const config = configChoice ?? state.config;
  const closeModal = (name: NonNullable<typeof modal>) => setModal(current => current === name ? null : current);
  const selectQuestion = (experience_id: string, question_index: number) =>
    change('/api/select', {experience_id, question_index}, true);
  const conversation = contextKey(state);
  const caseTurns = state.turns.filter(turn => turn.experience_id === state.experience_id);
  const turns = caseTurns.filter(turn => turn.question_index === state.question_index);
  const latest = turns[turns.length - 1];
  const visited = new Set(caseTurns.map(turn => turn.question_index));
  const count = state.scenario.questions.length;
  const currentSend = pending.current.get(conversation);
  const showPending = currentSend && (sending || recovering.current === currentSend.request.request_id);

  return <>
    <WorkbenchShell
      header={<>
        <div className="brand-line">
          <span className="brand-name">Cognita Reply</span>
          <span className="header-divider" aria-hidden="true"/>
          <h1 className="product-name">Interview Arena</h1>
          <span className="bootcamp-label">Innovator Bootcamp 2026</span>
        </div>
        <div className="header-actions">
          <button type="button" className="native-button ghost small" onClick={() => setModal('lab')}><Icon name="gear" size={16}/> Settings</button>
          <button type="button" className="native-button ghost small" onClick={() => setModal('guide')}>Guide</button>
        </div>
      </>}
      session={<>
        <div className="experience-control">
          <label htmlFor="experience-select">Practice case</label>
          <select id="experience-select" value={state.experience_id} disabled={locked} onChange={event => selectQuestion(event.target.value, 0)}>
            {state.scenario.experiences.map(item => <option value={item.id} key={item.id}>{item.title}</option>)}
          </select>
        </div>
        <div className="question-navigation" role="group" aria-label="Case questions">
          <span className="question-navigation-label">Questions</span>
          {state.scenario.questions.map((question, index) => <button key={index} type="button"
            aria-label={'Question ' + (index + 1) + (visited.has(index) ? ' (with messages): ' : ': ') + question}
            aria-current={index === state.question_index ? 'step' : undefined}
            data-current={index === state.question_index || undefined}
            data-visited={visited.has(index) || undefined}
            title={question} disabled={locked}
            onClick={() => {if (index !== state.question_index) void selectQuestion(state.experience_id, index);}}
            >{index + 1}{visited.has(index) && <span className="question-activity" aria-hidden="true"/>}</button>)}
        </div>
      </>}
      dossier={<div className="dossier-document">
        <div className="dossier-heading"><h2 className="dossier-title">Case facts</h2></div>
        <div className="dossier-scroll-frame">
          <div ref={facts} className="dossier-scroll" key={state.session_id + ':' + state.experience_id} tabIndex={0} role="region" aria-label="Full case facts">
            <div><div className="dossier-facts"><CaseFacts facts={state.scenario.experience} experienceId={state.experience_id}/></div>
              <details className="dossier-details"><summary>Interview context</summary><h3>{state.scenario.title}</h3><p className="dossier-role">{state.scenario.role}</p><p>{state.scenario.context}</p></details>
            </div>
          </div>
          {moreFacts && <div className="dossier-scroll-more" aria-hidden="true"><Icon name="chevron-down" size={16}/></div>}
        </div>
      </div>}
      conversation={<Conversation
        conversationKey={conversation}
        question={state.question}
        questionLabel={'Question ' + (state.question_index + 1) + ' of ' + count}
        turns={turns}
        pending={showPending ? {id: currentSend.request.request_id, text: currentSend.request.answer} : undefined}
        busy={busy}
        elapsed={elapsed}
        error={error || feedbackError || state.error || ''}
        retry={Boolean(feedbackError && currentSend?.request.answer === draft.trim())}
        draft={draft}
        disabled={changing || Boolean(state.error)}
        onEdit={edit}
        onSend={submit}
        onContinue={latest && state.question_index + 1 < count
          ? () => void selectQuestion(state.experience_id, state.question_index + 1)
          : undefined}
      />}
    />
    <div className="sr-only" role="status">{notice}</div>
    <Modal open={modal === 'guide'} title="Guide" onClose={() => closeModal('guide')}>
      <GuideContent onOpenSettings={() => setModal('lab')}/>
    </Modal>
    <Modal open={modal === 'lab'} title="Settings" onClose={() => closeModal('lab')}>
      <div className="lab-field">
        <label htmlFor="config">Coach configuration</label>
        <div className="lab-config">
          <select id="config" value={config} disabled={locked} onChange={event => setConfigChoice(event.target.value)}><option value="base">Base</option><option value="demo">Demo</option></select>
          <button type="button" className="native-button secondary" disabled={locked} onClick={() => change('/api/config', {name: config})}>Load</button>
        </div>
        <p>You’re using the {state.config === 'demo' ? 'Demo' : 'Base'} configuration. Loading a configuration clears the conversation.</p>
        <details className="lab-source"><summary>Configuration files</summary><p>Edit <code>coach.md</code> or <code>scenario.json</code> in the configuration folder, then select Load.</p><small>Loaded version {state.config_revision}</small></details>
      </div>
      <div className="lab-field">
        <label className="checkbox-label"><input id="thinking" type="checkbox" checked={thinking} disabled={locked} onChange={event => setThinking(event.target.checked)}/>Reasoning</label>
        <p>The model runs on your computer and keeps its internal reasoning hidden.</p>
      </div>
      {latest && <details className="lab-metrics"><summary>Latest reply details</summary><dl>
        <div><dt>Elapsed time</dt><dd>{latest.elapsed_seconds.toFixed(1)} s</dd></div>
        <div><dt>Tokens generated</dt><dd>{latest.usage.completion_tokens}</dd></div>
        <div><dt>Reasoning</dt><dd>{latest.thinking ? 'On' : 'Off'}</dd></div>
      </dl></details>}
      <div className="session-actions">
        <h3>Session</h3><p>Both actions clear the conversation.</p>
        <div className="footer-actions">
          <button type="button" className="native-button secondary small" disabled={locked} onClick={() => void change('/api/reset', {}).then(changed => {if (changed) setModal(null);})}>Start over</button>
          <button type="button" className="native-button ghost small" disabled={locked} onClick={() => void shutdown()}>Stop the app</button>
        </div>
      </div>
      {error && <p className="error-banner" role="alert">{error}</p>}
    </Modal>
  </>;
}
