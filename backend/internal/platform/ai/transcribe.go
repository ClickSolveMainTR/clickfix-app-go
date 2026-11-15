package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

const MaxAudioBytes = 25 << 20

type AudioRequest struct {
	Audio           []byte
	Filename        string
	Model           string
	DurationSeconds int64
}
type TranscriptSegment struct {
	Start   float64 `json:"start"`
	End     float64 `json:"end"`
	Speaker string  `json:"speaker,omitempty"`
	Text    string  `json:"text"`
}
type Transcript struct {
	Text     string              `json:"text"`
	Language string              `json:"language,omitempty"`
	Duration float64             `json:"duration"`
	Segments []TranscriptSegment `json:"segments"`
}
type Transcriber interface {
	Transcribe(context.Context, AudioRequest) (Transcript, error)
}

// Transcribe adapts otopoly-go's voice/client.go multipart contract (sent by openai-go since CF-213). Never replay
// transport failures: the provider may already have accepted a billable request.
func (p *HTTPProvider) Transcribe(ctx context.Context, in AudioRequest) (Transcript, error) {
	if p.Kind != ProviderOpenAI || p.Key == "" || in.Model == "" {
		return Transcript{}, ErrNotConfigured
	}
	if len(in.Audio) == 0 || len(in.Audio) > MaxAudioBytes {
		return Transcript{}, ErrInvalidOutput
	}
	name := in.Filename
	if name == "" {
		name = "recording.mp4"
	}
	format := "verbose_json"
	if strings.Contains(in.Model, "diarize") {
		format = "diarized_json"
	} else if strings.HasPrefix(in.Model, "gpt-") {
		format = "json"
	}
	// One attempt: an upload is never replayed, even after a 429.
	ctx = withCall(ctx, call{model: in.Model, attempts: 1})
	var resp *http.Response
	_, err := p.openaiTranscriptions().New(ctx, openai.AudioTranscriptionNewParams{
		File: openai.File(bytes.NewReader(in.Audio), name, "application/octet-stream"), Model: openai.AudioModel(in.Model),
		ResponseFormat: openai.AudioResponseFormat(format)}, option.WithResponseBodyInto(&resp))
	if err != nil {
		return Transcript{}, providerError(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out Transcript
	if err = json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Transcript{}, ErrInvalidOutput
	}
	if strings.TrimSpace(out.Text) == "" || len(out.Text) > 100000 || math.IsNaN(out.Duration) || math.IsInf(out.Duration, 0) || out.Duration < 0 || out.Duration > 86400 {
		return Transcript{}, ErrInvalidOutput
	}
	if out.Segments == nil {
		out.Segments = []TranscriptSegment{}
	}
	for _, seg := range out.Segments {
		if seg.Start < 0 || seg.End < seg.Start || seg.End > 86400 || math.IsNaN(seg.Start) || math.IsNaN(seg.End) {
			return Transcript{}, ErrInvalidOutput
		}
	}
	return out, nil
}
