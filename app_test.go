package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The integration fixture remains in .cache: this repository never permanently deletes files.
func appFixture(t *testing.T) string {
	t.Helper()
	if err := os.MkdirAll(".cache", 0755); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(".cache", "http-fixture-")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"base", "demo"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0755); err != nil {
			t.Fatal(err)
		}
		for _, filename := range []string{"coach.md", "scenario.json"} {
			data, err := os.ReadFile(filepath.Join("configs", name, filename))
			if err != nil {
				t.Fatal(err)
			}
			// Keep these compatibility tests on a legacy single-experience fixture,
			// even when the distributed configuration uses multiple experiences.
			if filename == "scenario.json" {
				var scenario interviewScenario
				if err := json.Unmarshal(data, &scenario); err != nil {
					t.Fatal(err)
				}
				if len(scenario.Experiences) > 0 {
					first := scenario.Experiences[0]
					data, err = json.Marshal(map[string]any{"title": scenario.Title, "role": scenario.Role, "context": scenario.Context, "experience": first.Facts, "questions": first.Questions})
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := os.WriteFile(filepath.Join(root, name, filename), data, 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return root
}

func appRequest(t *testing.T, client *http.Client, endpoint, method, payload, host, origin, contentType string) (int, map[string]any) {
	t.Helper()
	r, err := http.NewRequest(method, endpoint, strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if host != "" {
		r.Host = host
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	resp, err := client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, body
}

func TestAppHTTPRealFlowAndFailureBoundaries(t *testing.T) {
	fixture := appFixture(t)
	var mode atomic.Int32
	requests := make(chan map[string]any, 12)
	entered, release := make(chan struct{}, 1), make(chan struct{})
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("engine authorization missing")
		}
		// Context preflight is independent of the completion/error modes below.
		switch r.URL.Path {
		case "/apply-template":
			appJSON(w, 200, map[string]any{"prompt": "legacy fixture prompt"})
			return
		case "/tokenize":
			appJSON(w, 200, map[string]any{"tokens": []int{1, 2, 3}})
			return
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		requests <- request
		content, finish := "<think>private reasoning</think>Explain your personal contribution.", "stop"
		switch mode.Load() {
		case 1:
			content, finish = "Incomplete feedback", "length"
		case 2:
			content = "<think>unfinished internal reasoning"
		case 3:
			entered <- struct{}{}
			<-release
		}
		appJSON(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content, "reasoning_content": "must never be exposed"}, "finish_reason": finish}}, "usage": coachUsage{PromptTokens: 123, CompletionTokens: 45, TotalTokens: 168}})
	}))
	defer engine.Close()
	handler, err := newApp(engine.URL, "test-key", fixture, nil)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	client := server.Client()
	post := func(path, payload string) (int, map[string]any) {
		return appRequest(t, client, server.URL+path, "POST", payload, "", "", "application/json")
	}
	status, state := appRequest(t, client, server.URL+"/api/state", "GET", "", "", "", "")
	if status != 200 || state["busy"] != false {
		t.Fatalf("unexpected initial state: %d %v", status, state)
	}
	initialRevision := state["config_revision"]
	status, _ = appRequest(t, client, server.URL+"/api/state", "GET", "", "attacker.example", "", "")
	if status != 403 {
		t.Fatalf("DNS rebinding host was accepted: %d", status)
	}
	status, _ = appRequest(t, client, server.URL+"/api/reset", "POST", "{}", "", "https://attacker.example", "application/json")
	if status != 403 {
		t.Fatalf("cross-origin mutation was accepted: %d", status)
	}
	status, _ = appRequest(t, client, server.URL+"/api/reset", "POST", "{}", "", "", "text/plain")
	if status != 415 {
		t.Fatalf("simple cross-origin content type was accepted: %d", status)
	}

	status, result := post("/api/answer", `{"answer":"We worked together.","thinking":true,"seed":42}`)
	if status != 200 || result["finish_reason"] != "stop" || result["feedback"] != "Explain your personal contribution." {
		t.Fatalf("unexpected feedback: %d %v", status, result)
	}
	encoded, _ := json.Marshal(result)
	if bytes.Contains(encoded, []byte("private reasoning")) || bytes.Contains(encoded, []byte("must never")) {
		t.Fatal("reasoning leaked")
	}
	request := <-requests
	// Inspect the first real HTTP engine request: later questions must not steer this turn.
	var sentConversation strings.Builder
	for _, message := range request["messages"].([]any) {
		sentConversation.WriteString(message.(map[string]any)["content"].(string))
		sentConversation.WriteByte('\n')
	}
	if !strings.Contains(sentConversation.String(), state["question"].(string)) {
		t.Fatal("current interview question missing from engine request")
	}
	questions := state["scenario"].(map[string]any)["questions"].([]any)
	for _, futureQuestion := range questions[1:] {
		if strings.Contains(sentConversation.String(), futureQuestion.(string)) {
			t.Fatalf("future interview question leaked into first engine request: %s", futureQuestion)
		}
	}
	if request["reasoning_budget_tokens"] != float64(coachReasoningBudget) || request["seed"] != float64(42) || request["max_tokens"] != float64(coachMaxTokens) {
		t.Fatalf("wrong engine request: %v", request)
	}
	if request["chat_template_kwargs"].(map[string]any)["enable_thinking"] != true {
		t.Fatal("thinking not enabled")
	}

	status, result = post("/api/answer", `{"answer":"I prepared the interview guide.","thinking":false}`)
	if status != 200 {
		t.Fatalf("retry failed: %v", result)
	}
	request = <-requests
	messages := request["messages"].([]any)
	if len(messages) != 4 || !strings.Contains(messages[2].(map[string]any)["content"].(string), "personal contribution") {
		t.Fatal("retry lost previous feedback")
	}

	mode.Store(1)
	status, result = post("/api/answer", `{"answer":"An interrupted attempt.","thinking":true}`)
	if status != 502 || result["finish_reason"] != "length" || result["partial_feedback"] != "Incomplete feedback" {
		t.Fatalf("truncation accepted or lost: %v", result)
	}
	<-requests
	mode.Store(2)
	status, result = post("/api/answer", `{"answer":"An attempt without a final reply.","thinking":true}`)
	if status != 502 || result["partial_feedback"] != "" {
		t.Fatalf("unfinished reasoning accepted: %v", result)
	}
	<-requests
	_, state = appRequest(t, client, server.URL+"/api/state", "GET", "", "", "", "")
	if state["busy"] != false || len(state["turns"].([]any)) != 2 {
		t.Fatalf("failed feedback altered session: %v", state)
	}

	mode.Store(3)
	done := make(chan int, 1)
	go func() {
		r, e := client.Post(server.URL+"/api/answer", "application/json", strings.NewReader(`{"answer":"An answer during slow generation.","thinking":true}`))
		if e != nil {
			done <- 0
			return
		}
		defer r.Body.Close()
		_, _ = io.Copy(io.Discard, r.Body)
		done <- r.StatusCode
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("engine did not start")
	}
	status, _ = post("/api/reset", "{}")
	if status != 409 {
		t.Fatalf("busy session reset: %d", status)
	}
	close(release)
	if status = <-done; status != 200 {
		t.Fatalf("busy generation failed: %d", status)
	}
	<-requests

	_, state = appRequest(t, client, server.URL+"/api/state", "GET", "", "", "", "")
	previousSession := state["session_id"]
	scenarioPath := filepath.Join(fixture, "base", "scenario.json")
	scenarioData, err := os.ReadFile(scenarioPath)
	if err != nil {
		t.Fatal(err)
	}
	var scenario map[string]any
	if err := json.Unmarshal(scenarioData, &scenario); err != nil {
		t.Fatal(err)
	}
	scenario["experience"] = "The candidate tested an accessibility map with two students."
	scenarioData, err = json.Marshal(scenario)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scenarioPath, scenarioData, 0644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(fixture, "base", "coach.md")
	data, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	data = append(data, []byte("\nTest edit: give a single suggestion.\n")...)
	if e = os.WriteFile(path, data, 0644); e != nil {
		t.Fatal(e)
	}
	status, state = post("/api/config", `{"name":"base"}`)
	if status != 200 || state["config_revision"] == initialRevision || state["session_id"] == previousSession || len(state["turns"].([]any)) != 0 {
		t.Fatalf("reload failed: %v", state)
	}

	if state["scenario"].(map[string]any)["experience"] != scenario["experience"] {
		t.Fatalf("reload did not expose the edited scenario: %v", state)
	}
	status, _ = post("/api/answer", `{"answer":"We reviewed an accessibility map."}`)
	if status != 200 {
		t.Fatalf("answer after reload failed: %d", status)
	}
	payload, err := json.Marshal(<-requests)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"Test edit: give a single suggestion.", scenario["experience"].(string)} {
		if !bytes.Contains(payload, []byte(text)) {
			t.Fatalf("engine request did not use the reloaded content: %s", text)
		}
	}
	status, _ = post("/api/config", `{"name":"../../outside"}`)
	if status != 400 {
		t.Fatal("unexpected configuration path accepted")
	}
	status, state = post("/api/next", "{}")
	if status != 200 || state["question_index"] != float64(1) {
		t.Fatalf("next question failed: %v", state)
	}
}

// A malformed edit must leave the already loaded scenario and question usable.
func TestAppConfigInvalidReloadKeepsPrevious(t *testing.T) {
	fixture := appFixture(t)
	handler, err := newApp("http://127.0.0.1:1", "unused", fixture, nil)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	client := server.Client()
	status, previous := appRequest(t, client, server.URL+"/api/next", "POST", "{}", "", "", "application/json")
	if status != 200 {
		t.Fatalf("cannot prepare loaded scenario: %v", previous)
	}
	path := filepath.Join(fixture, "base", "scenario.json")
	validJSON, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []struct{ field, value, expected string }{
		{"contex", "misspelled field", "unrecognised field"},
		{"context", "   ", "context"},
		{"experience", "", "experience"},
	} {
		t.Run(change.field, func(t *testing.T) {
			var edited map[string]any
			if err := json.Unmarshal(validJSON, &edited); err != nil {
				t.Fatal(err)
			}
			edited[change.field] = change.value
			invalidJSON, err := json.Marshal(edited)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, invalidJSON, 0644); err != nil {
				t.Fatal(err)
			}
			status, result := appRequest(t, client, server.URL+"/api/config", "POST", `{"name":"base"}`, "", "", "application/json")
			message, _ := result["error"].(string)
			if status != 400 || !strings.Contains(message, change.expected) {
				t.Fatalf("invalid scenario accepted or unclear error: %d %v", status, result)
			}
			_, current := appRequest(t, client, server.URL+"/api/state", "GET", "", "", "", "")
			for _, key := range []string{"config_revision", "config", "question", "question_index"} {
				if current[key] != previous[key] {
					t.Fatalf("bad reload changed %s: %v -> %v", key, previous[key], current[key])
				}
			}
		})
	}
}
