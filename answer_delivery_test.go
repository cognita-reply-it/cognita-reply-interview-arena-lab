package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// Drive the public handlers and all three engine requests without loading a model.
type deliveryEngine struct {
	completions int
	prompt      []coachMessage
	maxHistory  int
	failOnce    bool
	entered     chan struct{}
	release     chan struct{}
}

func (e *deliveryEngine) RoundTrip(r *http.Request) (*http.Response, error) {
	body := ""
	if r.URL.Path == "/apply-template" || r.URL.Path == "/v1/chat/completions" {
		var request struct {
			Messages []coachMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			return nil, err
		}
		e.prompt = request.Messages
	}
	switch r.URL.Path {
	case "/apply-template":
		body = `{"prompt":"fictional scenario"}`
	case "/tokenize":
		body = `{"tokens":[1,2]}`
		if e.maxHistory > 0 && (len(e.prompt)-2)/2 > e.maxHistory {
			body = `{"tokens":[` + strings.Repeat("0,", 8191) + `0]}`
		}
	case "/v1/chat/completions":
		e.completions++
		if e.entered != nil {
			close(e.entered)
			<-e.release
		}
		if e.failOnce {
			e.failOnce = false
			return nil, fmt.Errorf("simulated engine interruption")
		}
		body = `{"choices":[{"message":{"content":"Your contribution is clear."},"finish_reason":"stop"}]}`
	default:
		return nil, fmt.Errorf("unexpected engine path %s", r.URL.Path)
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
}

func deliveryApp(t *testing.T) (*arenaApp, *deliveryEngine) {
	t.Helper()
	return deliveryAppWithConfigs(t, "configs")
}

func deliveryAppWithConfigs(t *testing.T, configRoot string) (*arenaApp, *deliveryEngine) {
	t.Helper()
	handler, err := newApp("http://engine.invalid", "", configRoot, nil)
	if err != nil {
		t.Fatal(err)
	}
	app := handler.(*arenaApp)
	engine := &deliveryEngine{}
	app.client.Transport = engine
	return app, engine
}

func deliveryCall(t *testing.T, handler http.Handler, path string, payload map[string]any, wantStatus int) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, deliveryRequest(t, path, payload))
	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if rec.Code != wantStatus {
		t.Fatalf("%s: got %d, want %d (%v)", path, rec.Code, wantStatus, result["error"])
	}
	return result
}

func deliveryRequest(t *testing.T, path string, payload map[string]any) *http.Request {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	method := http.MethodPost
	if payload == nil {
		method = http.MethodGet
	}
	req := httptest.NewRequest(method, "http://127.0.0.1"+path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func deliveryContext(state map[string]any) map[string]any {
	return map[string]any{"session_id": state["session_id"], "experience_id": state["experience_id"], "question_index": state["question_index"]}
}

func TestAnswerRejectsStaleContext(t *testing.T) {
	for _, mutation := range []string{"/api/next", "/api/select", "/api/reset", "/api/config"} {
		t.Run(mutation, func(t *testing.T) {
			configRoot := "configs"
			if mutation == "/api/select" {
				configRoot = "testdata/coach-regression"
			}
			app, engine := deliveryAppWithConfigs(t, configRoot)
			original := deliveryCall(t, app, "/api/state", nil, 200)
			payload := map[string]any{}
			switch mutation {
			case "/api/select":
				experiences := original["scenario"].(map[string]any)["experiences"].([]any)
				payload = map[string]any{"experience_id": experiences[1].(map[string]any)["id"], "question_index": 0}
			case "/api/config":
				payload["name"] = "demo"
			}
			deliveryCall(t, app, mutation, payload, 200)
			input := map[string]any{"answer": "I collected the notes.", "request_id": "first-tab", "expected_context": deliveryContext(original)}
			rejected := deliveryCall(t, app, "/api/answer", input, 409)
			if rejected["code"] != "context_changed" || engine.completions != 0 || len(app.state.Turns) != 0 {
				t.Fatalf("stale answer was not rejected before generation: %v", rejected["code"])
			}
		})
	}
}

type interruptedResponse struct{ *httptest.ResponseRecorder }

func (*interruptedResponse) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestAnswerRetryAfterLostResponse(t *testing.T) {
	app, engine := deliveryApp(t)
	first := deliveryCall(t, app, "/api/answer", map[string]any{"answer": "I collected the notes."}, 200)
	input := map[string]any{"answer": "We reviewed them together.", "message_kind": "clarification", "request_id": "detail", "expected_context": deliveryContext(first["state"].(map[string]any))}
	// The server commits but writing the response body to the browser fails.
	lost := &interruptedResponse{httptest.NewRecorder()}
	app.ServeHTTP(lost, deliveryRequest(t, "/api/answer", input))
	if lost.Code != 200 || lost.Body.Len() != 0 {
		t.Fatal("failed to simulate a lost successful response")
	}
	replay := deliveryCall(t, app, "/api/answer", input, 200)
	again := deliveryCall(t, app, "/api/answer", input, 200)
	if engine.completions != 2 || len(app.state.Turns) != 2 || len(app.state.Clarifications) != 1 {
		t.Fatal("identical retry changed the memory or number of engine calls")
	}
	if !reflect.DeepEqual(replay, again) || replay["feedback"] != app.state.Turns[1].Feedback || replay["elapsed_seconds"] != app.state.Turns[1].ElapsedSeconds {
		t.Fatal("identical retry changed the receipt")
	}
	deliveryCall(t, app, "/api/answer", map[string]any{"answer": "Which fact matters?", "message_kind": "question"}, 200)
	replay = deliveryCall(t, app, "/api/answer", input, 200)
	if len(replay["state"].(map[string]any)["turns"].([]any)) != 3 || engine.completions != 3 {
		t.Fatal("replay returned a stale session snapshot or reran generation")
	}
	for key, value := range map[string]any{"answer": "Different text", "message_kind": "answer", "thinking": true, "seed": 42} {
		previous, exists := input[key]
		input[key] = value
		conflict := deliveryCall(t, app, "/api/answer", input, 409)
		if conflict["code"] != "request_id_conflict" {
			t.Errorf("same ID with different %s: wrong conflict code", key)
		}
		if exists {
			input[key] = previous
		} else {
			delete(input, key)
		}
	}
	if engine.completions != 3 {
		t.Fatal("conflicting retry was sent to the engine")
	}
}

func TestLegacyRetryCannotReachNewSession(t *testing.T) {
	app, engine := deliveryApp(t)
	input := map[string]any{"answer": "I collected the notes.", "request_id": "old-client"}
	deliveryCall(t, app, "/api/answer", input, 200)
	deliveryCall(t, app, "/api/reset", map[string]any{}, 200)
	result := deliveryCall(t, app, "/api/answer", input, 409)
	if result["code"] != "request_id_conflict" || engine.completions != 1 || len(app.state.Turns) != 0 {
		t.Fatal("legacy request ID was reused in a new session")
	}
	previousFilter := app.retiredRequestIDs
	fresh := map[string]any{"answer": "New session.", "request_id": "new-id"}
	deliveryCall(t, app, "/api/answer", fresh, 200)
	deliveryCall(t, app, "/api/reset", map[string]any{}, 200)
	if previousFilter == nil || previousFilter != app.retiredRequestIDs || !previousFilter.contains("new-id") || !previousFilter.contains("old-client") || len(app.completedAnswers) != 0 {
		t.Fatal("session reset retained receipts, forgot a used ID, or grew the retired ID filter")
	}
	fresh["expected_context"] = deliveryContext(deliveryCall(t, app, "/api/state", nil, 200))
	deliveryCall(t, app, "/api/answer", fresh, 200)
}

func TestAnswerConcurrentDelivery(t *testing.T) {
	app, engine := deliveryApp(t)
	engine.entered, engine.release = make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(engine.release) })
	defer unblock()
	input := map[string]any{"answer": "I collected the notes.", "request_id": "single-submission", "expected_context": deliveryContext(deliveryCall(t, app, "/api/state", nil, 200))}
	req := deliveryRequest(t, "/api/answer", input)
	first := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		app.ServeHTTP(first, req)
		close(done)
	}()
	select {
	case <-engine.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("engine did not start")
	}
	deliveryCall(t, app, "/api/answer", input, 409)
	deliveryCall(t, app, "/api/next", map[string]any{}, 409)
	if s := deliveryCall(t, app, "/api/state", nil, 200); s["busy"] != true || len(s["turns"].([]any)) != 0 {
		t.Fatal("in-flight delivery published an unfinished turn")
	}
	unblock()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("answer did not finish")
	}
	result := deliveryCall(t, app, "/api/answer", input, 200)
	if first.Code != 200 || result["state"].(map[string]any)["busy"] != false || engine.completions != 1 || len(app.state.Turns) != 1 {
		t.Fatal("overlapping retries produced multiple turns or left a busy session")
	}
}

func TestAnswerRetryAfterEngineFailureWithCapture(t *testing.T) {
	app, engine := deliveryApp(t)
	if err := os.MkdirAll(".local-tests", 0700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(".local-tests", "request-retry-")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.setCaptureDir(dir); err != nil {
		t.Fatal(err)
	}
	engine.failOnce = true
	input := map[string]any{"answer": "I collected the notes.", "request_id": "retry"}
	deliveryCall(t, app, "/api/answer", input, 502)
	deliveryCall(t, app, "/api/answer", input, 200)
	entries, err := filepath.Glob(filepath.Join(dir, "*.request.json"))
	if err != nil || len(entries) != 2 || engine.completions != 2 || len(app.state.Turns) != 1 {
		t.Fatalf("retry did not preserve both attempt captures and exactly one turn: %v", err)
	}
}

func TestShutdownMakesSessionUnavailable(t *testing.T) {
	app, engine := deliveryApp(t)
	stale := deliveryCall(t, app, "/api/state", nil, 200)
	active := deliveryCall(t, app, "/api/reset", map[string]any{}, 200)
	deliveryCall(t, app, "/api/answer", map[string]any{"answer": "I collected the notes."}, 200)
	stopped := deliveryCall(t, app, "/api/shutdown", map[string]any{}, 200)
	if stopped["session_id"] == stale["session_id"] || stopped["session_id"] != active["session_id"] {
		t.Fatal("shutdown did not identify the session actually stopped")
	}
	for _, path := range []string{"/api/state", "/api/answer", "/api/select", "/api/reset", "/api/config", "/api/next"} {
		payload := map[string]any{}
		if path == "/api/state" {
			payload = nil
		}
		if path == "/api/answer" {
			payload["answer"] = "Another message."
		}
		result := deliveryCall(t, app, path, payload, 410)
		if result["code"] != "session_stopped" || result["session_id"] != stopped["session_id"] || result["turns"] != nil {
			t.Fatalf("%s still exposes the session", path)
		}
	}
	replay := deliveryCall(t, app, "/api/shutdown", map[string]any{}, 200)
	if replay["session_id"] != stopped["session_id"] || engine.completions != 1 || len(app.state.Turns) != 0 || len(app.conversations) != 0 || len(app.completedAnswers) != 0 || app.retiredRequestIDs != nil {
		t.Fatal("shutdown retained interview data or allowed a new generation")
	}
}

func TestChatPreservesDialogueWithinQuestionAndTokenBudget(t *testing.T) {
	app, engine := deliveryAppWithConfigs(t, "testdata/coach-regression")
	state := deliveryCall(t, app, "/api/state", nil, 200)
	context := deliveryContext(state)
	var latest map[string]any
	for i := 0; i < 12; i++ {
		latest = map[string]any{"message_kind": "chat", "answer": fmt.Sprintf("Message %d", i), "request_id": fmt.Sprintf("chat-%d", i), "expected_context": context}
		deliveryCall(t, app, "/api/answer", latest, 200)
	}
	// A retry of a completed chat turn replays the same reply.
	deliveryCall(t, app, "/api/answer", latest, 200)
	if engine.completions != 12 || len(app.state.Turns) != 12 || len(engine.prompt) != 22 {
		t.Fatal("wrong number of exchanges or wrong chat window")
	}
	if system := engine.prompt[0]; system.Role != "system" || !strings.Contains(system.Content, state["question"].(string)) {
		t.Fatal("opening interview question missing from chat context")
	}
	for i := 0; i < 10; i++ {
		human, coach := engine.prompt[2*i+1], engine.prompt[2*i+2]
		if human.Role != "user" || human.Content != fmt.Sprintf("Message %d", i+1) || coach.Role != "assistant" || coach.Content != app.state.Turns[i+1].Feedback {
			t.Fatalf("exchange %d was changed or attributed to the wrong speaker", i+1)
		}
	}
	if last := engine.prompt[21]; last.Role != "user" || last.Content != "Message 11" {
		t.Fatal("latest user input was rewritten")
	}

	engine.maxHistory = 2
	deliveryCall(t, app, "/api/answer", map[string]any{"message_kind": "chat", "answer": "Why?"}, 200)
	if len(engine.prompt) != 6 || engine.prompt[1].Content != "Message 10" || engine.prompt[5].Content != "Why?" || app.state.ActiveAnswer != "" {
		t.Fatal("trimming lost recent dialogue or a chat question became a factual statement")
	}
	experiences := app.state.Scenario.Experiences
	for _, target := range []map[string]any{
		{"experience_id": experiences[0].ID, "question_index": 1},
		{"experience_id": experiences[1].ID, "question_index": 0},
	} {
		deliveryCall(t, app, "/api/select", target, 200)
		deliveryCall(t, app, "/api/answer", map[string]any{"message_kind": "chat", "answer": "Where should I start?"}, 200)
		if len(engine.prompt) != 2 || strings.Contains(engine.prompt[0].Content, "Message 11") {
			t.Fatal("chat leaked another question's dialogue")
		}
	}
	if strings.Contains(engine.prompt[0].Content, "clickable prototype") || !strings.Contains(engine.prompt[0].Content, "paper noticeboard") {
		t.Fatal("chat received the wrong scenario facts")
	}
	deliveryCall(t, app, "/api/select", map[string]any{"experience_id": experiences[0].ID, "question_index": 0}, 200)
	deliveryCall(t, app, "/api/answer", map[string]any{"message_kind": "chat", "answer": "Let's continue."}, 200)
	if len(engine.prompt) != 6 || engine.prompt[3].Content != "Why?" {
		t.Fatal("reopening a question did not restore its chat")
	}
}

func TestCoachInstructionsSelectOnlyActiveMode(t *testing.T) {
	plain := "# Custom coach\r\n\r\n## Tone\r\nBe precise.\r\n"
	sectioned := plain + "## Practice mode\r\nRUBRIC\r\n## Chat mode\r\nDIALOGUE\r\n"
	for _, kind := range []string{"chat", "answer", "clarification", "question", "revision"} {
		if got := coachInstructions(plain, kind); got != plain {
			t.Errorf("plain custom coach changed in %s mode", kind)
		}
		got := coachInstructions(sectioned, kind)
		if !strings.HasPrefix(got, plain) || strings.Contains(got, "DIALOGUE") != (kind == "chat") || strings.Contains(got, "RUBRIC") != (kind != "chat") {
			t.Errorf("wrong section for %s: %q", kind, got)
		}
	}

	app, engine := deliveryApp(t)
	for _, config := range []string{"base", "demo"} {
		deliveryCall(t, app, "/api/config", map[string]any{"name": config}, 200)
		for _, kind := range []string{"chat", "answer"} {
			deliveryCall(t, app, "/api/answer", map[string]any{"message_kind": kind, "answer": "It helped us."}, 200)
			system := engine.prompt[0].Content
			if !strings.Contains(system, "Reply in English") || strings.Contains(system, "respond with an open-ended question") != (kind == "chat") || strings.Contains(system, "100 words") != (kind == "answer") {
				t.Fatalf("%s coach supplied the wrong instructions to generation for %s", config, kind)
			}
		}
	}
}

func TestReplyDeadlineUsesConfiguredLimit(t *testing.T) {
	app, _ := deliveryApp(t)
	app.client.Timeout = time.Nanosecond
	result := deliveryCall(t, app, "/api/answer", map[string]any{
		"answer": "I collected the interview notes.", "message_kind": "chat",
	}, http.StatusGatewayTimeout)
	if !strings.Contains(fmt.Sprint(result["error"]), app.client.Timeout.String()) {
		t.Fatalf("unexpected timeout message: %v", result["error"])
	}
	if len(app.state.Turns) != 0 || app.state.Busy {
		t.Fatal("timeout changed the conversation or left it busy")
	}
	app.client.Timeout = time.Minute
	deliveryCall(t, app, "/api/answer", map[string]any{
		"answer": "I collected the interview notes.", "message_kind": "chat",
	}, http.StatusOK)
}
