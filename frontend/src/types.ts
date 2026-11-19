type MessageKind = 'chat' | 'answer' | 'revision' | 'clarification' | 'question';

export interface InterviewTurn {
  request_id: string;
  experience_id: string;
  question_index: number;
  question: string;
  answer: string;
  feedback: string;
  message_kind: MessageKind;
  active_answer: string;
  clarifications: string[];
  thinking: boolean;
  elapsed_seconds: number;
  usage: {prompt_tokens: number; completion_tokens: number; total_tokens: number};
}

export interface AnswerContext {
  session_id: string;
  experience_id: string;
  question_index: number;
}

export interface AnswerRequest {
  expected_context: AnswerContext;
  request_id: string;
  message_kind: 'chat';
  answer: string;
  thinking: boolean;
}

export interface InterviewState extends AnswerContext {
  active_answer: string;
  clarifications: string[];
  candidate_memory: {question_index: number; question: string; active_answer: string; clarifications: string[]}[];
  config: string;
  config_revision: string;
  loaded_at: string;
  scenario: {
    title: string;
    role: string;
    context: string;
    experience: string;
    questions: string[];
    experiences: {id: string; title: string; facts: string; questions: string[]}[];
  };
  question: string;
  turns: InterviewTurn[];
  busy: boolean;
  error?: string;
}

export interface AnswerResult {
  state: InterviewState;
  feedback: string;
  elapsed_seconds: number;
}
