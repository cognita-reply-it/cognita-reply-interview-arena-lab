package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// A configuration describes one person. A new session separates another person.
type interviewExperience struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Facts     string   `json:"facts"`
	Questions []string `json:"questions"`
}
type candidateContext struct {
	ActiveAnswer   string   `json:"active_answer"`
	Clarifications []string `json:"clarifications"`
	EventNumber    int      `json:"event_number"`
	MessageKind    string   `json:"message_kind"`
	RequestID      string   `json:"request_id"`
}
type candidateMemory struct {
	QuestionIndex  int      `json:"question_index"`
	Question       string   `json:"question"`
	ActiveAnswer   string   `json:"active_answer"`
	Clarifications []string `json:"clarifications"`
}
type interviewConversation struct {
	Context candidateContext
	Turns   []interviewTurn
}
type answerInput struct {
	Answer          string           `json:"answer"`
	Thinking        bool             `json:"thinking"`
	Seed            *int             `json:"seed,omitempty"`
	MessageKind     string           `json:"message_kind"`
	RequestID       string           `json:"request_id"`
	ExpectedContext *expectedContext `json:"expected_context,omitempty"`
}

type expectedContext struct {
	SessionID     string `json:"session_id"`
	ExperienceID  string `json:"experience_id"`
	QuestionIndex *int   `json:"question_index"`
}

// The current session, experience and question are checked separately under the app lock.
func (in answerInput) fingerprint() [sha256.Size]byte {
	in.ExpectedContext = nil
	encoded, _ := json.Marshal(in)
	return sha256.Sum256(encoded)
}

type promptDetails struct {
	Method                   string `json:"method"`
	PromptTokens             int    `json:"prompt_tokens"`
	ReservedCompletionTokens int    `json:"reserved_completion_tokens"`
	SafetyTokens             int    `json:"safety_tokens"`
	ContextLimit             int    `json:"context_limit"`
	HistoryAvailable         int    `json:"history_turns_available"`
	HistoryUsed              int    `json:"history_turns_used"`
	CandidateMemoryItems     int    `json:"candidate_memory_items"`
}

var safeIdentifier = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)

const maxSessionTurns = 200

// Legacy clients do not bind a retry to a session. A process-wide Bloom filter
// remembers retired legacy IDs without storing text or growing with sessions.
// A rare false positive asks a legacy client for a new ID; there are no false
// negatives. Clients that send expected_context never use this filter.
type retiredRequestFilter [1 << 16]uint64 // 512 KiB, allocated on the first legacy reset

func retiredIDBits(id string) [6]uint32 {
	hash := sha256.Sum256([]byte(id))
	var bits [6]uint32
	for i := range bits {
		bits[i] = binary.LittleEndian.Uint32(hash[4*i:]) % uint32(64 * len(retiredRequestFilter{}))
	}
	return bits
}

func (f *retiredRequestFilter) contains(id string) bool {
	if f == nil || id == "" {
		return false
	}
	for _, bit := range retiredIDBits(id) {
		if f[bit/64]&(uint64(1)<<(bit%64)) == 0 {
			return false
		}
	}
	return true
}

func (f *retiredRequestFilter) add(id string) {
	for _, bit := range retiredIDBits(id) {
		f[bit/64] |= uint64(1) << (bit % 64)
	}
}

func emptyCandidateContext() candidateContext { return candidateContext{Clarifications: []string{}} }
func randomIdentifier() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
func normalizeScenario(s *interviewScenario) error {
	if strings.TrimSpace(s.Context) == "" {
		return errors.New("scenario.json: context is required and cannot be empty")
	}
	if strings.TrimSpace(s.Title) == "" || strings.TrimSpace(s.Role) == "" {
		return errors.New("scenario.json: title and role cannot be empty")
	}
	if len(s.Experiences) == 0 {
		if strings.TrimSpace(s.Experience) == "" {
			return errors.New("scenario.json: experience is required and cannot be empty")
		}
		s.Experiences = []interviewExperience{{ID: "default", Title: s.Title, Facts: s.Experience, Questions: s.Questions}}
	} else if s.Experience != "" || len(s.Questions) > 0 {
		return errors.New("scenario.json: choose either experiences or experience/questions")
	}
	if len(s.Experiences) > 10 {
		return errors.New("scenario.json: specify at most 10 experiences")
	}
	seen := map[string]bool{}
	for _, e := range s.Experiences {
		if !safeIdentifier.MatchString(e.ID) || seen[e.ID] {
			return errors.New("scenario.json: each experience needs a unique alphanumeric ID. Underscores and hyphens are allowed")
		}
		seen[e.ID] = true
		if strings.TrimSpace(e.Title) == "" || strings.TrimSpace(e.Facts) == "" {
			return fmt.Errorf("scenario.json: experience %s is missing title or facts", e.ID)
		}
		if len(e.Questions) == 0 || len(e.Questions) > 20 {
			return fmt.Errorf("scenario.json: experience %s needs between 1 and 20 questions", e.ID)
		}
		for _, q := range e.Questions {
			if strings.TrimSpace(q) == "" {
				return errors.New("scenario.json: questions cannot be empty")
			}
		}
	}
	return nil
}
func conversationKey(experience string, question int) string {
	return fmt.Sprintf("%s/%d", experience, question)
}
func (a *arenaApp) currentConversation() *interviewConversation {
	return a.conversations[conversationKey(a.state.ExperienceID, a.state.QuestionIndex)]
}

// Caller holds a.mu. Selection never modifies or discards an existing conversation.
func (a *arenaApp) selectConversation(experience string, question int) error {
	var selected *interviewExperience
	for i := range a.state.Scenario.Experiences {
		if a.state.Scenario.Experiences[i].ID == experience {
			selected = &a.state.Scenario.Experiences[i]
			break
		}
	}
	if selected == nil {
		return errors.New("experience unavailable")
	}
	if question < 0 || question >= len(selected.Questions) {
		return errors.New("invalid question index")
	}
	a.state.ExperienceID = experience
	a.state.QuestionIndex = question
	a.state.Question = selected.Questions[question]
	// Legacy fields always describe the active experience.
	a.state.Scenario.Experience = selected.Facts
	a.state.Scenario.Questions = selected.Questions
	key := conversationKey(experience, question)
	if a.conversations[key] == nil {
		a.conversations[key] = &interviewConversation{Context: emptyCandidateContext(), Turns: []interviewTurn{}}
	}
	a.refreshCandidateState()
	return nil
}
func (a *arenaApp) refreshCandidateState() {
	a.state.candidateContext = a.currentConversation().Context
	a.state.CandidateMemory = []candidateMemory{}
	for index := range a.state.Scenario.Questions {
		if index == a.state.QuestionIndex {
			continue
		}
		c := a.conversations[conversationKey(a.state.ExperienceID, index)]
		if c == nil || c.Context.ActiveAnswer == "" {
			continue
		}
		a.state.CandidateMemory = append(a.state.CandidateMemory, candidateMemory{index, a.state.Scenario.Questions[index], c.Context.ActiveAnswer, append([]string{}, c.Context.Clarifications...)})
	}
}
func (a *arenaApp) newSession() error {
	id, err := randomIdentifier()
	if err != nil {
		return err
	}
	a.state.SessionID = id
	a.state.Turns = []interviewTurn{}
	a.conversations = map[string]*interviewConversation{}
	for requestID, receipt := range a.completedAnswers {
		if receipt.legacy {
			if a.retiredRequestIDs == nil {
				a.retiredRequestIDs = new(retiredRequestFilter)
			}
			a.retiredRequestIDs.add(requestID)
		}
	}
	a.completedAnswers = map[string]completedAnswer{}
	return a.selectConversation(a.state.Scenario.Experiences[0].ID, 0)
}
func (a *arenaApp) prepareCandidate(in *answerInput) (candidateContext, error) {
	if in.MessageKind != "chat" && in.MessageKind != "answer" && in.MessageKind != "clarification" && in.MessageKind != "revision" && in.MessageKind != "question" {
		return candidateContext{}, errors.New("message_kind must be chat, answer, clarification, revision or question")
	}
	if len(a.state.Turns) >= maxSessionTurns {
		return candidateContext{}, errors.New("This session has reached 200 exchanges. Your history is still available. Start a new session to continue.")
	}
	if in.RequestID == "" {
		var err error
		in.RequestID, err = randomIdentifier()
		if err != nil {
			return candidateContext{}, err
		}
	}
	if !safeIdentifier.MatchString(in.RequestID) {
		return candidateContext{}, errors.New("invalid request_id: use 1–100 letters, digits, underscores or hyphens")
	}
	next := a.currentConversation().Context
	next.Clarifications = append([]string{}, next.Clarifications...)
	if next.ActiveAnswer == "" && (in.MessageKind == "clarification" || in.MessageKind == "revision") {
		return candidateContext{}, errors.New("Write an answer first or choose Ask the coach.")
	}
	next.MessageKind = in.MessageKind
	next.RequestID = in.RequestID
	next.EventNumber++
	switch in.MessageKind {
	case "answer":
		next.ActiveAnswer = in.Answer
		next.Clarifications = []string{}
	case "revision":
		next.ActiveAnswer = in.Answer
	case "clarification":
		next.Clarifications = append(next.Clarifications, in.Answer)
	}
	if len(next.Clarifications) > 20 {
		return candidateContext{}, errors.New("You have added 20 clarifications. Write a new answer that incorporates them before adding more.")
	}
	return next, nil
}

const conversationSemantics = "\n\nHOW TO INTERPRET MESSAGES\nUse the current answer, clarifications and available history to interpret the latest event. Scenario facts are reference material. Only attribute statements to the participant if they actually made them. A clarification adds to the active answer. A revision replaces it. Memory from other questions contains earlier statements about this same experience. Treat earlier feedback as history, separate from participant statements and current instructions. For a question event, answer the participant's question directly."

// The preamble is shared. A named section applies up to the next mode heading.
// A plain coach.md, including one with custom headings, applies in full.
func coachInstructions(coach, kind string) string {
	var selected []string
	active := true
	for _, line := range strings.Split(coach, "\n") {
		switch strings.TrimSpace(line) {
		case "## Chat mode", "## Modalità chat":
			active = kind == "chat"
		case "## Practice mode", "## Modalità esercizio":
			active = kind != "chat"
		default:
			if active {
				selected = append(selected, line)
			}
		}
	}
	return strings.Join(selected, "\n")
}

func candidatePrompt(question, event string, c candidateContext, memory []candidateMemory) string {
	instruction := "Review the active answer and its clarifications against the interview question and available facts. You may confirm it is adequate as written."
	if c.MessageKind == "question" {
		instruction = "Answer my question about interview preparation using the available context. Treat it as a request for help."
	}
	payload := struct {
		Question       string            `json:"current_question"`
		Active         string            `json:"active_candidate_answer"`
		Clarifications []string          `json:"related_clarifications"`
		Memory         []candidateMemory `json:"candidate_memory_from_other_questions,omitempty"`
		Event          struct {
			Kind   string `json:"kind"`
			Text   string `json:"text"`
			Number int    `json:"number"`
		} `json:"current_event"`
		Instruction string `json:"current_request"`
	}{Question: question, Active: c.ActiveAnswer, Clarifications: c.Clarifications, Memory: memory, Instruction: instruction}
	payload.Event.Kind = c.MessageKind
	payload.Event.Text = event
	payload.Event.Number = c.EventNumber
	raw, _ := json.Marshal(payload)
	return string(raw)
}

// Tokenization uses the same local model, conversation template and thinking flag
// as generation. No candidate text is shortened to fit the context.
var errContextTooLarge = errors.New("The context exceeds the model's capacity. Shorten the answer, clarifications or experience facts. No text was truncated or saved. Your history is still available.")

func (a *arenaApp) engineJSON(ctx context.Context, path string, body, result any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.engineURL+path, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if a.engineKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.engineKey)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %d", path, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(result)
}
func (a *arenaApp) countPromptTokens(ctx context.Context, messages []coachMessage, thinking bool) (int, error) {
	var template struct {
		Prompt string `json:"prompt"`
	}
	if err := a.engineJSON(ctx, "/apply-template", map[string]any{"messages": messages, "add_generation_prompt": true, "chat_template_kwargs": map[string]any{"enable_thinking": thinking}}, &template); err != nil {
		return 0, err
	}
	if template.Prompt == "" {
		return 0, errors.New("empty engine template")
	}
	var tokenized struct {
		Tokens []int `json:"tokens"`
	}
	if err := a.engineJSON(ctx, "/tokenize", map[string]any{"content": template.Prompt, "add_special": false, "parse_special": true}, &tokenized); err != nil {
		return 0, err
	}
	if len(tokenized.Tokens) == 0 {
		return 0, errors.New("empty token count")
	}
	return len(tokenized.Tokens), nil
}
func (a *arenaApp) composeMessages(ctx context.Context, system, question, event string, c candidateContext, memory []candidateMemory, turns []interviewTurn, thinking bool) ([]coachMessage, promptDetails, error) {
	available := len(turns)
	historyLimit := 2
	currentMessage := event
	if c.MessageKind == "chat" {
		historyLimit = 10
		system += "\n\nYou opened the interview with this question: " + question + "\nContinue the conversation by replying to the participant's latest message."
		memory = nil // Other questions have their own dialogue.
	} else {
		system += conversationSemantics
		currentMessage = candidatePrompt(question, event, c, memory)
	}
	if len(turns) > historyLimit {
		turns = turns[len(turns)-historyLimit:]
	}
	details := promptDetails{Method: "llama_cpp_tokenizer", ReservedCompletionTokens: coachMaxTokens, SafetyTokens: 128, ContextLimit: 8192, HistoryAvailable: available, CandidateMemoryItems: len(memory)}
	for {
		messages := []coachMessage{{Role: "system", Content: system}}
		for _, t := range turns {
			human := t.Answer
			if c.MessageKind != "chat" && t.MessageKind != "chat" {
				human = candidatePrompt(t.Question, t.Answer, t.candidateContext, nil)
			}
			messages = append(messages, coachMessage{Role: "user", Content: human}, coachMessage{Role: "assistant", Content: t.Feedback})
		}
		messages = append(messages, coachMessage{Role: "user", Content: currentMessage})
		details.HistoryUsed = len(turns)
		tokens, err := a.countPromptTokens(ctx, messages, thinking)
		details.PromptTokens = tokens
		if err != nil {
			return nil, details, err
		}
		if tokens+details.ReservedCompletionTokens+details.SafetyTokens <= details.ContextLimit {
			return messages, details, nil
		}
		if len(turns) == 0 {
			return nil, details, errContextTooLarge
		}
		// Only the oldest actual exchange is omitted; candidate facts stay intact.
		turns = turns[1:]
	}
}

func (a *arenaApp) setCaptureDir(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	private := func(p string) bool {
		for _, part := range strings.Split(p, string(os.PathSeparator)) {
			if part == ".local-tests" {
				return true
			}
		}
		return false
	}
	if !private(absolute) {
		return errors.New("capture-dir must be inside .local-tests")
	}
	if err = os.MkdirAll(absolute, 0700); err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return err
	}
	if !private(resolved) {
		return errors.New("capture-dir resolves outside .local-tests")
	}
	a.captureDir = resolved
	return nil
}
func writeCaptureExclusive(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, w := f.Write(data)
	s := f.Sync()
	c := f.Close()
	if w != nil {
		return w
	}
	if s != nil {
		return s
	}
	return c
}
func (a *arenaApp) captureRequest(c candidateContext, session, experience, revision string, question int, details promptDetails, encoded []byte) error {
	if a.captureDir == "" {
		return nil
	}
	// Keep each attempt as evidence, including retries after an engine failure.
	base := filepath.Join(a.captureDir, c.RequestID)
	err := writeCaptureExclusive(base+".request.json", encoded)
	if errors.Is(err, os.ErrExist) {
		suffix, randomErr := randomIdentifier()
		if randomErr != nil {
			return randomErr
		}
		base += ".retry-" + suffix
		err = writeCaptureExclusive(base+".request.json", encoded)
	}
	if err != nil {
		return err
	}
	sum := sha256.Sum256(encoded)
	meta := map[string]any{"request_id": c.RequestID, "session_id": session, "experience_id": experience, "question_index": question, "message_kind": c.MessageKind, "event_number": c.EventNumber, "config_revision": revision, "request_sha256": hex.EncodeToString(sum[:]), "request_bytes": len(encoded), "endpoint": "/v1/chat/completions", "captured_at": time.Now().UTC().Format(time.RFC3339Nano), "history_turns_used": details.HistoryUsed, "context": details}
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return writeCaptureExclusive(base+".meta.json", append(raw, '\n'))
}
