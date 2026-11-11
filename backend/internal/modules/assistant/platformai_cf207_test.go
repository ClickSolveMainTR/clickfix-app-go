package assistant

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
)

// scriptedTurn is one fake model turn of the CF-207 scenarios (#150).
type scriptedTurn struct {
	text  string
	calls []ai.ToolCall
	stop  string
	err   error
}

// turnsProvider plays scripted turns and records every request it saw.
type turnsProvider struct {
	turns []scriptedTurn
	seen  []ai.Request
}

func (p *turnsProvider) Generate(ctx context.Context, req ai.Request) (ai.Result, error) {
	return p.Stream(ctx, req, func(string) error { return nil })
}

func (p *turnsProvider) Stream(_ context.Context, req ai.Request, emit ai.Delta) (ai.Result, error) {
	p.seen = append(p.seen, req)
	if len(p.turns) == 0 {
		return ai.Result{FinalUsage: true, InputTokens: 1, OutputTokens: 1, StopReason: "end_turn", Calls: []ai.ToolCall{}}, nil
	}
	t := p.turns[0]
	p.turns = p.turns[1:]
	if t.text != "" && emit != nil {
		// Two chunks: the stream guard holds the last word back until flush.
		half := len(t.text) / 2
		for _, chunk := range []string{t.text[:half], t.text[half:]} {
			if err := emit(chunk); err != nil {
				return ai.Result{}, err
			}
		}
	}
	calls := t.calls
	if calls == nil {
		calls = []ai.ToolCall{}
	}
	stop := t.stop
	if stop == "" {
		stop = "end_turn"
		if len(calls) > 0 {
			stop = "tool_use"
		}
	}
	return ai.Result{FinalUsage: true, InputTokens: 10, OutputTokens: 5, StopReason: stop, Text: t.text, Calls: calls}, t.err
}

func toolCall(id, name, input string) ai.ToolCall {
	return ai.ToolCall{ID: id, Name: name, Input: json.RawMessage(input)}
}

func lookupTools() []ai.Tool {
	obj := map[string]any{"type": "object"}
	return []ai.Tool{{Name: "channels_list", Schema: obj}, {Name: "teams_list", Schema: obj}, {Name: "workflows_list", Schema: obj},
		{Name: "automations_list", Schema: obj}, {Name: ToolSearchDocs, Schema: obj}, suggestionTool()}
}

// Scenario 1: "list my channels, teams, workflows and automations" makes the
// model call four tools in one turn. Every call runs and the answer follows,
// instead of ErrTooManyRounds (#150).
func TestPlatformAIRunsParallelToolCalls(t *testing.T) {
	p := &turnsProvider{turns: []scriptedTurn{
		{calls: []ai.ToolCall{toolCall("c1", "channels_list", `{}`), toolCall("c2", "teams_list", `{}`), toolCall("c3", "workflows_list", `{}`), toolCall("c4", "automations_list", `{}`)}},
		{text: "Kanallar: 2, ekipler: 1, iş akışları: 3, otomasyonlar: 0."},
	}}
	pa := &PlatformAI{Routes: map[string]ai.Route{"reply": {Provider: p, ProviderName: "test", Model: "m"}}}
	var ran []string
	runner := func(_ context.Context, c ai.ToolCall) (ai.ToolResult, error) {
		ran = append(ran, c.Name)
		return toolJSON(map[string]any{"items": []any{}}), nil
	}
	var streamed strings.Builder
	res, err := pa.Run(context.Background(), Call{Route: "reply", Rounds: toolRounds, Runner: runner,
		Req:  ai.Request{Turns: []ai.Turn{{Role: "user", Text: "Kanallarımı, ekiplerimi, iş akışlarımı ve otomasyonlarımı listele"}}, Tools: lookupTools()},
		Emit: func(s string) error { streamed.WriteString(s); return nil }})
	if err != nil {
		t.Fatalf("parallel tool turn failed: %v", err)
	}
	if len(ran) != 4 || len(res.Calls) != 4 {
		t.Fatalf("ran=%v calls=%d", ran, len(res.Calls))
	}
	if !strings.Contains(streamed.String(), "Kanallar: 2") || res.Text != streamed.String() {
		t.Fatalf("answer not streamed: %q / %q", streamed.String(), res.Text)
	}
	// Four calls were one tool round: the model saw the user turn, then the tool results.
	if len(p.seen) != 2 || len(p.seen[1].Turns) != 1+1+4 {
		t.Fatalf("turns seen: %d / %d", len(p.seen), len(p.seen[1].Turns))
	}
}

// When the tool rounds are used up the model gets a last turn without tools
// and answers with what it has; no error reaches the user.
func TestPlatformAIFinishesAfterRoundLimit(t *testing.T) {
	var turns []scriptedTurn
	for i := 0; i < toolRounds; i++ {
		turns = append(turns, scriptedTurn{calls: []ai.ToolCall{toolCall("c", "channels_list", `{}`)}})
	}
	turns = append(turns, scriptedTurn{text: "Şimdiye kadar bulduklarım: ..."})
	p := &turnsProvider{turns: turns}
	pa := &PlatformAI{Routes: map[string]ai.Route{"reply": {Provider: p, ProviderName: "test", Model: "m"}}}
	runner := func(context.Context, ai.ToolCall) (ai.ToolResult, error) { return toolJSON(map[string]any{}), nil }
	res, err := pa.Run(context.Background(), Call{Route: "reply", Rounds: toolRounds, Runner: runner,
		Req:  ai.Request{Turns: []ai.Turn{{Role: "user", Text: "x"}}, Tools: lookupTools()},
		Emit: func(string) error { return nil }})
	if err != nil || !strings.HasPrefix(res.Text, "Şimdiye kadar") {
		t.Fatalf("err=%v text=%q", err, res.Text)
	}
	last := p.seen[len(p.seen)-1]
	if last.ToolChoice != "none" || last.Turns[len(last.Turns)-1].Role != "user" {
		t.Fatalf("last turn must forbid tools and ask for the answer: %+v", last.ToolChoice)
	}
}

// Scenario 2: the first turn stops on max_tokens. The reply continues in a
// second turn inside the same stream; no error event, both parts arrive.
func TestPlatformAIContinuesTruncatedAnswer(t *testing.T) {
	p := &turnsProvider{turns: []scriptedTurn{
		{text: "Birinci bölüm burada kesildi", stop: "max_tokens", err: ai.ErrTruncated},
		{text: " ve ikinci bölümle tamamlandı."},
	}}
	pa := &PlatformAI{Routes: map[string]ai.Route{"reply": {Provider: p, ProviderName: "test", Model: "m"}}}
	var streamed strings.Builder
	res, err := pa.Run(context.Background(), Call{Route: "reply", Rounds: toolRounds,
		Req:  ai.Request{Turns: []ai.Turn{{Role: "user", Text: "Uzun anlat"}}, Tools: lookupTools(), MaxTokens: chatMaxTokens},
		Emit: func(s string) error { streamed.WriteString(s); return nil }})
	if err != nil {
		t.Fatalf("truncated turn surfaced as an error: %v", err)
	}
	want := "Birinci bölüm burada kesildi ve ikinci bölümle tamamlandı."
	if streamed.String() != want || res.Text != want || res.Truncated {
		t.Fatalf("streamed=%q text=%q truncated=%v", streamed.String(), res.Text, res.Truncated)
	}
	second := p.seen[1]
	if second.Turns[len(second.Turns)-1].Role != "user" || second.Turns[len(second.Turns)-2].Text != "Birinci bölüm burada kesildi" {
		t.Fatalf("continuation must follow the cut-off assistant turn: %+v", second.Turns)
	}
	// Usage of every turn is kept.
	if res.InputTokens != 20 || res.OutputTokens != 10 {
		t.Fatalf("usage %d/%d", res.InputTokens, res.OutputTokens)
	}
}

// A reply still cut off after two continuations keeps its text and is marked
// truncated instead of failing; the service adds the "shortened" note.
func TestPlatformAIKeepsTextAfterRepeatedTruncation(t *testing.T) {
	cut := scriptedTurn{text: "parça ", stop: "max_tokens", err: ai.ErrTruncated}
	p := &turnsProvider{turns: []scriptedTurn{cut, cut, cut, {text: "asla"}}}
	pa := &PlatformAI{Routes: map[string]ai.Route{"reply": {Provider: p, ProviderName: "test", Model: "m"}}}
	res, err := pa.Run(context.Background(), Call{Route: "reply", Rounds: toolRounds,
		Req:  ai.Request{Turns: []ai.Turn{{Role: "user", Text: "x"}}, Tools: lookupTools()},
		Emit: func(string) error { return nil }})
	if err != nil || !res.Truncated || res.Text != "parça parça parça " || len(p.seen) != 3 {
		t.Fatalf("err=%v truncated=%v text=%q turns=%d", err, res.Truncated, res.Text, len(p.seen))
	}
	if truncatedNote("tr") == "" || truncatedNote("en") == "" || truncatedNote("tr") == truncatedNote("en") {
		t.Fatal("truncated note must exist in both languages")
	}
}

// Scenario 3: after search_docs the model calls suggest_replies without any
// prose. The call is refused without ending the turn; the model then writes
// the answer and the suggestions are saved. The fallback text never shows.
func TestPlatformAIRequiresTextAfterLookup(t *testing.T) {
	p := &turnsProvider{turns: []scriptedTurn{
		{calls: []ai.ToolCall{toolCall("s1", ToolSearchDocs, `{"query":"kanal"}`)}},
		{calls: []ai.ToolCall{toolCall("q1", suggestionToolName, `{"replies":["Devam"]}`)}},
		{text: "Belgeye göre kanal şöyle kurulur.", calls: []ai.ToolCall{toolCall("q2", suggestionToolName, `{"replies":["Kanal kur","Test et"]}`)}},
	}}
	pa := &PlatformAI{Routes: map[string]ai.Route{"reply": {Provider: p, ProviderName: "test", Model: "m"}}}
	var streamed strings.Builder
	var suggestions []string
	runner := func(_ context.Context, c ai.ToolCall) (ai.ToolResult, error) {
		if c.Name == suggestionToolName {
			replies, result := suggestionResult(c.Input, strings.TrimSpace(streamed.String()) != "")
			if !result.IsError {
				suggestions = replies
			}
			return result, nil
		}
		return toolJSON(map[string]any{"guides": []any{map[string]any{"title": "Kanallar"}}}), nil
	}
	res, err := pa.Run(context.Background(), Call{Route: "reply", Rounds: toolRounds, Runner: runner,
		Req:  ai.Request{Turns: []ai.Turn{{Role: "user", Text: "Kanal nasıl kurulur?"}}, Tools: lookupTools()},
		Emit: func(s string) error { streamed.WriteString(s); return nil }})
	if err != nil {
		t.Fatalf("lookup reply failed: %v", err)
	}
	if res.Text != "Belgeye göre kanal şöyle kurulur." || streamed.String() != res.Text {
		t.Fatalf("answer text missing: %q", res.Text)
	}
	if len(suggestions) != 2 || len(res.Calls) != 3 || len(p.seen) != 3 {
		t.Fatalf("suggestions=%v calls=%d turns=%d", suggestions, len(res.Calls), len(p.seen))
	}
	// The refusal reached the model as a tool error on the second call.
	third := p.seen[2]
	last := third.Turns[len(third.Turns)-1]
	if last.Role != "tool" || !strings.Contains(last.Text, "write your markdown answer") {
		t.Fatalf("refusal not passed to the model: %+v", last)
	}
}

// A card-only reply (#115) may still save suggestions without prose.
func TestSuggestionResultKeepsCardOnlyRule(t *testing.T) {
	replies, result := suggestionResult(json.RawMessage(`{"replies":["Onayla"]}`), true)
	if result.IsError || len(replies) != 1 {
		t.Fatalf("card-only suggestions refused: %+v", result)
	}
	if _, result = suggestionResult(json.RawMessage(`{"replies":["Onayla"]}`), false); !result.IsError {
		t.Fatal("suggestions without an answer accepted")
	}
	if _, result = suggestionResult(json.RawMessage(`nope`), true); !result.IsError {
		t.Fatal("invalid suggestions accepted")
	}
}

// The gate of suggest_replies ignores text streamed before a lookup: an
// intro ("let me check the docs…") followed by search_docs and a bare
// suggest_replies still forces the model to write the answer (#150).
func TestStreamRunnerIgnoresIntroBeforeLookup(t *testing.T) {
	p := &turnsProvider{turns: []scriptedTurn{
		{text: "Belgelere bakıyorum…", calls: []ai.ToolCall{toolCall("s1", "channels_list", `{}`)}},
		{calls: []ai.ToolCall{toolCall("q1", suggestionToolName, `{"replies":["Devam"]}`)}},
		{text: " Kanallar şöyle listelenir.", calls: []ai.ToolCall{toolCall("q2", suggestionToolName, `{"replies":["Kanal kur"]}`)}},
	}}
	pa := &PlatformAI{Routes: map[string]ai.Route{"reply": {Provider: p, ProviderName: "test", Model: "m"}}}
	var events []string
	emit := func(name string, data any) error {
		if name == "delta" {
			events = append(events, data.(map[string]string)["text"])
		}
		return nil
	}
	stream := &safeStream{emit: emit}
	run := &toolRun{locale: "tr"} // channels_list is not offered here: "unknown tool" error, still a lookup
	res, err := pa.Run(context.Background(), Call{Route: "reply", Rounds: toolRounds, Runner: streamRunner(stream, run, emit, "tr"),
		Req:  ai.Request{Turns: []ai.Turn{{Role: "user", Text: "Kanallarımı listele"}}, Tools: lookupTools()},
		Emit: stream.add})
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.flush(); err != nil {
		t.Fatal(err)
	}
	if len(run.suggestions) != 1 || run.suggestions[0] != "Kanal kur" || len(res.Calls) != 3 {
		t.Fatalf("suggestions=%v calls=%d", run.suggestions, len(res.Calls))
	}
	if got := strings.Join(events, ""); got != "Belgelere bakıyorum… Kanallar şöyle listelenir." {
		t.Fatalf("streamed %q", got)
	}
	if last := p.seen[2].Turns[len(p.seen[2].Turns)-1]; last.Role != "tool" || !strings.Contains(last.Text, "write your markdown answer") {
		t.Fatalf("intro text opened the gate: %+v", last)
	}
}
