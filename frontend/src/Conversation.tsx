import {Fragment, useLayoutEffect, useRef, type FormEvent} from 'react';
import {MessageSquare} from 'lucide-react';
import {Icon} from '../vendor/stageshow/kit/parts/Icon';
import {Spinner} from '../vendor/stageshow/kit/parts/Spinner';
import type {InterviewTurn} from './types';
import './conversation.css';

interface ConversationProps {
  conversationKey: string;
  question: string;
  questionLabel: string;
  turns: InterviewTurn[];
  pending?: {id: string; text: string};
  busy: boolean;
  elapsed: number;
  error: string;
  retry: boolean;
  draft: string;
  disabled: boolean;
  onEdit: (value: string) => void;
  onSend: (event: FormEvent<HTMLFormElement>) => void;
  onContinue?: () => void;
}

function CoachText({text}: {text: string}) {
  // Coach configurations may use bold labels. Treat all other output as text.
  return <p>{text.split(/(\*\*[^*\n]+\*\*)/).map((part, index) =>
    index % 2 ? <strong key={index}>{part.slice(2, -2)}</strong> : <Fragment key={index}>{part}</Fragment>,
  )}</p>;
}

function UserMessage({text}: {text: string}) {
  return <div className="chat-message chat-user">
    <span className="message-speaker">You</span>
    <div className="message-bubble"><p>{text}</p></div>
  </div>;
}

export function Conversation({
  conversationKey, question, questionLabel, turns, pending, busy, elapsed, error, retry, draft, disabled, onEdit, onSend, onContinue,
}: ConversationProps) {
  const viewport = useRef<HTMLDivElement>(null);
  const newestReply = useRef<HTMLDivElement>(null);
  const following = useRef(true);
  const previousConversation = useRef('');
  const lastTurnId = turns[turns.length - 1]?.request_id;
  const pendingId = pending?.id;
  useLayoutEffect(() => {
    const scroll = viewport.current;
    if (!scroll) return;
    const changedConversation = previousConversation.current !== conversationKey;
    previousConversation.current = conversationKey;
    if (changedConversation || pendingId) following.current = true;
    if (!following.current) return;
    const response = busy ? null : newestReply.current;
    const startOfResponse = response
      ? scroll.scrollTop + response.getBoundingClientRect().top - scroll.getBoundingClientRect().top - 12
      : scroll.scrollHeight;
    scroll.scrollTop = Math.min(scroll.scrollHeight - scroll.clientHeight, startOfResponse);
  }, [conversationKey, lastTurnId, pendingId, busy]);

  const messages = turns.flatMap(turn => [
    <UserMessage key={turn.request_id + ':user'} text={turn.answer}/>,
    <div className="chat-message chat-coach" key={turn.request_id + ':coach'}
      ref={turn.request_id === lastTurnId ? newestReply : undefined}>
      <span className="message-speaker">Coach</span>
      <div className="message-bubble"><CoachText text={turn.feedback}/></div>
    </div>,
  ]);
  if (pending) messages.push(<UserMessage key={pending.id + ':user'} text={pending.text}/>);

  return <>
    <header className="chat-header">
      <div className="chat-identity">
        <span className="chat-avatar" aria-hidden="true"><MessageSquare size={19} strokeWidth={1.8}/></span>
        <div>
          <h2 id="chat-heading" tabIndex={-1}>Interview coach</h2>
          <p>{questionLabel} · Local model</p>
        </div>
      </div>
      {onContinue && <button className="native-button ghost small chat-continue" type="button"
        onClick={onContinue} disabled={disabled || busy}>
        Next question <Icon name="arrow" size={14}/>
      </button>}
    </header>

    <div ref={viewport} className="chat-scroll" role="region" tabIndex={0} aria-label="Conversation with the coach"
      onScroll={event => {
        const {scrollTop, scrollHeight, clientHeight} = event.currentTarget;
        following.current = scrollHeight - clientHeight - scrollTop < 96;
      }}>
      <div key={conversationKey} className="chat-thread" role="log" aria-label="Messages">
        <div className="chat-message chat-coach chat-opening">
          <span className="message-speaker">Coach</span>
          <div className="message-bubble">
            <h3 id="question">{question || 'No question available. Check the scenario in Settings.'}</h3>
          </div>
        </div>
        {messages}
      </div>
      <div className="sr-only" role="status">{busy ? 'The coach is preparing a reply.' : ''}</div>
      {busy && <div className="chat-thread chat-wait" aria-hidden="true">
        <div className="chat-message chat-coach">
          <div className="message-bubble">
            <Spinner size={14}/> <span>Coach is replying</span>
            <span className="chat-time">{Math.floor(elapsed)} s</span>
          </div>
        </div>
      </div>}
    </div>

    <form className="chat-compose" id="answer-form" onSubmit={onSend}>
      {error && <p className="error-banner" role="alert">{error}</p>}
      <div className="chat-input-shell">
        <label className="sr-only" htmlFor="answer">Write to the coach</label>
        <textarea id="answer" className="chat-input" rows={2} maxLength={6000}
          value={pending ? '' : draft} onChange={event => onEdit(event.target.value)}
          onKeyDown={event => {
            if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing) {
              event.preventDefault();
              event.currentTarget.form?.requestSubmit();
            }
          }}
          aria-describedby="compose-hint" placeholder="Write to the coach…" readOnly={busy} disabled={disabled}/>
        <button id="send" type="submit" className="native-button primary chat-send" disabled={disabled || busy || !draft.trim()}>
          {retry ? 'Retry' : 'Send'} <Icon name="send" size={16}/>
        </button>
      </div>
      <div className="chat-input-meta">
        <span id="compose-hint">Enter to send · Shift+Enter for a new line</span>
        {draft.length >= 5000 && <span>{draft.length.toLocaleString('en-US')} / 6,000</span>}
      </div>
    </form>
  </>;
}
