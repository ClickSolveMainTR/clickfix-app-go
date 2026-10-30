package usecase

func objectSchema(properties map[string]any) map[string]any {
	required := make([]string, 0, len(properties))
	for key := range properties {
		required = append(required, key)
	}
	return map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
}
func textSchema(max int) map[string]any { return map[string]any{"type": "string", "maxLength": max} }
func enumSchema(values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": values}
}
func confidenceSchema() map[string]any {
	return map[string]any{"type": "number", "minimum": 0, "maximum": 1}
}
func stringsSchema(max int) map[string]any {
	return map[string]any{"type": "array", "maxItems": max, "items": textSchema(100)}
}

var triageSchema = makeTriageSchema()

func makeTriageSchema() map[string]any {
	schema := objectSchema(map[string]any{
		"language": textSchema(30), "intent": textSchema(100), "category": textSchema(100),
		"sentiment": enumSchema("positive", "neutral", "negative"), "urgency": enumSchema("low", "normal", "high", "critical"),
		"priority": enumSchema("low", "normal", "high", "urgent"), "summary": textSchema(2000),
		"suggested_tags": stringsSchema(10), "suggested_team": textSchema(100), "confidence": confidenceSchema(),
	})
	schema["properties"].(map[string]any)["suggested_assignee_uuid"] = map[string]any{"anyOf": []any{map[string]any{"type": "string", "format": "uuid"}, map[string]any{"type": "null"}}}
	return schema
}

var replySchema = objectSchema(map[string]any{
	"reply": textSchema(6000), "summary": textSchema(2000), "confidence": confidenceSchema(),
	"handoff":      map[string]any{"type": "boolean"},
	"citation_ids": map[string]any{"type": "array", "maxItems": 10, "items": map[string]any{"type": "string", "format": "uuid"}},
})

// copilotSchema keeps the composer's text free of headings, quotes and notes
// addressed to the agent: drafts are inserted into the reply box verbatim.
var copilotSchema = objectSchema(map[string]any{"text": textSchema(8000)})

const copilotOutputRule = `Put the result in "text". For draft operations (suggest_reply, rewrite, translate, tone, shorten, expand) "text" is ONLY the message to the customer, ready to send: no title, no label such as "Draft", no quote marks or ">" prefix, no notes to the agent, never mention knowledge sources, chunks or IDs. For summarize, next_action and ask, "text" is the answer to the agent in short plain paragraphs or "-" lists.`
