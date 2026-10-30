package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"strings"
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

// Transcribe adapts otopoly-go's voice/client.go multipart contract. Never replay
// transport failures: the provider may already have accepted a billable request.
func (p *HTTPProvider) Transcribe(ctx context.Context, in AudioRequest) (Transcript, error) {
	if p.Kind != ProviderOpenAI || p.Key == "" || in.Model == "" {
		return Transcript{}, ErrNotConfigured
	}
	if len(in.Audio) == 0 || len(in.Audio) > MaxAudioBytes {
		return Transcript{}, ErrInvalidOutput
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	name := in.Filename
	if name == "" {
		name = "recording.mp4"
	}
	part, err := mw.CreateFormFile("file", name)
	if err != nil {
		return Transcript{}, err
	}
	if _, err = part.Write(in.Audio); err != nil {
		return Transcript{}, err
	}
	format := "verbose_json"
	if strings.Contains(in.Model, "diarize") {
		format = "diarized_json"
	} else if strings.HasPrefix(in.Model, "gpt-") {
		format = "json"
	}
	for k, v := range map[string]string{"model": in.Model, "response_format": format} {
		if err = mw.WriteField(k, v); err != nil {
			return Transcript{}, err
		}
	}
	if err = mw.Close(); err != nil {
		return Transcript{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/audio/transcriptions", &buf)
	if err != nil {
		return Transcript{}, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+p.Key)
	resp, err := p.Client.Do(req)
	if err != nil {
		return Transcript{}, ErrUnavailable
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Transcript{}, &Rejected{Status: resp.StatusCode}
	}
	var out Transcript
	if err = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&out); err != nil {
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
