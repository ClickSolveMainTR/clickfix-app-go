package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

// openaiOptions builds the request options for the OpenAI-compatible
// services. The services are built directly (not through openai.NewClient)
// so no OPENAI_* environment value reaches a request.
func (p *HTTPProvider) openaiOptions() []option.RequestOption {
	return []option.RequestOption{
		option.WithEnvironmentProduction(),
		option.WithBaseURL(p.BaseURL + "/"),
		option.WithAPIKey(p.Key),
		option.WithHTTPClient(p.httpClient()),
		option.WithMaxRetries(0),
		option.WithRequestTimeout(p.timeout()),
		option.WithMiddleware(p.middleware),
	}
}

func (p *HTTPProvider) openaiChat() *openai.ChatCompletionService {
	s := openai.NewChatCompletionService(p.openaiOptions()...)
	return &s
}

func (p *HTTPProvider) openaiEmbeddings() *openai.EmbeddingService {
	s := openai.NewEmbeddingService(p.openaiOptions()...)
	return &s
}

func (p *HTTPProvider) openaiTranscriptions() *openai.AudioTranscriptionService {
	s := openai.NewAudioTranscriptionService(p.openaiOptions()...)
	return &s
}

func openaiParams(req Request, stream bool) openai.ChatCompletionNewParams {
	if req.MaxTokens <= 0 {
		req.MaxTokens = 2048
	}
	params := openai.ChatCompletionNewParams{Model: req.Model, MaxTokens: openai.Int(req.MaxTokens),
		Messages: []openai.ChatCompletionMessageParamUnion{openai.SystemMessage(req.Instructions)}}
	for _, turn := range req.Turns {
		switch turn.Role {
		case "tool":
			params.Messages = append(params.Messages, openai.ToolMessage(turn.Text, turn.ToolCallID))
		case "assistant":
			msg := openai.ChatCompletionAssistantMessageParam{}
			if turn.Text != "" || len(turn.Calls) == 0 {
				msg.Content.OfString = openai.String(turn.Text)
			}
			for _, c := range turn.Calls {
				msg.ToolCalls = append(msg.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
					ID: c.ID, Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{Name: c.Name, Arguments: string(c.Input)}}})
			}
			params.Messages = append(params.Messages, openai.ChatCompletionMessageParamUnion{OfAssistant: &msg})
		default:
			params.Messages = append(params.Messages, openai.UserMessage(turn.Text))
		}
	}
	for _, t := range req.Tools {
		params.Tools = append(params.Tools, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name: t.Name, Description: openai.String(t.Description), Parameters: shared.FunctionParameters(t.Schema)}))
	}
	if len(params.Tools) > 0 && req.ToolChoice == "none" {
		params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: openai.String("none")}
	}
	if len(req.Schema) > 0 {
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
			JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{Name: "answer", Strict: openai.Bool(true), Schema: anthropicSchema(req.Schema)}}}
	}
	if stream {
		params.StreamOptions = openai.ChatCompletionStreamOptionsParam{IncludeUsage: openai.Bool(true)}
	}
	return params
}

func (p *HTTPProvider) openaiGenerate(ctx context.Context, req Request) (Result, error) {
	cc, err := p.openaiChat().New(ctx, openaiParams(req, false))
	if err != nil {
		return Result{}, providerError(err)
	}
	out := Result{FinalUsage: true, Provider: p.Kind, Model: cc.Model, Calls: []ToolCall{},
		InputTokens: cc.Usage.PromptTokens, OutputTokens: cc.Usage.CompletionTokens}
	if len(cc.Choices) != 1 {
		return out, ErrInvalidOutput
	}
	choice := cc.Choices[0]
	out.StopReason = choice.FinishReason
	out.Text = choice.Message.Content
	if choice.Message.Refusal != "" {
		return out, ErrRefused
	}
	for _, c := range choice.Message.ToolCalls {
		out.Calls = append(out.Calls, ToolCall{ID: c.ID, Name: c.Function.Name, Input: json.RawMessage(c.Function.Arguments)})
	}
	return out, stopError(choice.FinishReason)
}

func (p *HTTPProvider) openaiStreamBody(ctx context.Context, req Request) (io.ReadCloser, error) {
	var resp *http.Response
	_, err := p.openaiChat().New(ctx, openaiParams(req, true), option.WithJSONSet("stream", true), option.WithResponseBodyInto(&resp))
	if err != nil {
		return nil, providerError(err)
	}
	return resp.Body, nil
}

// openaiChunk applies one chat completion chunk and returns its text delta.
func (s *streamState) openaiChunk(data string) (string, error) {
	var probe struct {
		Type string `json:"type"`
	}
	var chunk openai.ChatCompletionChunk
	if json.Unmarshal([]byte(data), &probe) != nil || json.Unmarshal([]byte(data), &chunk) != nil {
		return "", ErrInvalidOutput
	}
	if probe.Type == "error" {
		return "", ErrUnavailable
	}
	if chunk.Model != "" {
		s.out.Model = chunk.Model
	}
	if in, out := chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens; in > 0 || out > 0 {
		s.out.InputTokens = in
		s.out.OutputTokens = out
		s.out.FinalUsage = true
	}
	delta := ""
	for _, c := range chunk.Choices {
		delta += c.Delta.Content
		if c.Delta.Refusal != "" {
			s.terminal = ErrRefused
		}
		if err := stopError(c.FinishReason); err != nil {
			s.terminal = err
		}
		for _, wc := range c.Delta.ToolCalls {
			i := int(wc.Index)
			call := s.calls[i]
			if call == nil {
				call = &ToolCall{}
				s.calls[i] = call
			}
			call.ID += wc.ID
			call.Name += wc.Function.Name
			s.args[i] += wc.Function.Arguments
		}
	}
	return delta, nil
}

func (p *HTTPProvider) Embed(ctx context.Context, input []string) (Embeddings, error) {
	return p.EmbedModel(ctx, p.EmbeddingModel, input)
}

// EmbedModel embeds with the named model. text-embedding-3 models are asked
// for the fixed EmbeddingDimensions so every stored vector has one size.
func (p *HTTPProvider) EmbedModel(ctx context.Context, model string, input []string) (Embeddings, error) {
	if p.Kind != ProviderOpenAI || model == "" {
		return Embeddings{}, ErrUnavailable
	}
	if p.Key == "" {
		return Embeddings{}, ErrNotConfigured
	}
	ctx = withCall(ctx, call{model: model, limit: 16 << 20})
	params := openai.EmbeddingNewParams{Model: model,
		Input: openai.EmbeddingNewParamsInputUnion{OfArrayOfStrings: input}, EncodingFormat: openai.EmbeddingNewParamsEncodingFormatFloat}
	if strings.HasPrefix(model, "text-embedding-3") {
		params.Dimensions = openai.Int(EmbeddingDimensions)
	}
	res, err := p.openaiEmbeddings().New(ctx, params)
	if err != nil {
		return Embeddings{}, providerError(err)
	}
	if len(res.Data) != len(input) {
		return Embeddings{}, ErrInvalidOutput
	}
	out := make([][]float32, len(input))
	for _, d := range res.Data {
		if d.Index < 0 || int(d.Index) >= len(out) || out[d.Index] != nil || len(d.Embedding) == 0 {
			return Embeddings{}, ErrInvalidOutput
		}
		v := make([]float32, len(d.Embedding))
		for i, x := range d.Embedding {
			v[i] = float32(x)
		}
		out[d.Index] = v
	}
	tokens := res.Usage.PromptTokens
	if tokens == 0 {
		tokens = res.Usage.TotalTokens
	}
	return Embeddings{Vectors: out, InputTokens: tokens}, nil
}
