package ai

import "encoding/json"

// body returns the JSON the SDK sends for req, as a generic map (CF-213).
// The wire tests from the hand-written HTTP layer keep asserting on it.
func (p *HTTPProvider) body(req Request, stream bool) (string, map[string]any) {
	var params any = openaiParams(req, stream)
	path := "/chat/completions"
	if p.Kind == ProviderAnthropic {
		params = anthropicParams(req)
		path = "/messages"
	}
	raw, err := json.Marshal(params)
	if err != nil {
		panic(err)
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		panic(err)
	}
	if stream {
		out["stream"] = true
	}
	return path, out
}
