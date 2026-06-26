package common

import "github.com/QuantumNous/new-api/common"

// Fy-api overlay: Codex-style Responses payloads may include client-internal
// bookkeeping fields that OpenAI's public Responses schema rejects. These are
// safe to strip before forwarding because they are not part of the upstream API.
func SanitizeOpenAIResponsesRequest(jsonData []byte) ([]byte, error) {
	out, _, err := sanitizeOpenAIResponsesRequest(jsonData, false)
	return out, err
}

// Fy-api overlay: when upstream explicitly rejects stale encrypted state, retry
// once without those encrypted blocks so the request can degrade to plaintext
// context instead of hard-failing.
func StripEncryptedContentFromOpenAIResponsesRequest(jsonData []byte) ([]byte, bool, error) {
	return sanitizeOpenAIResponsesRequest(jsonData, true)
}

func sanitizeOpenAIResponsesRequest(jsonData []byte, stripEncrypted bool) ([]byte, bool, error) {
	var data map[string]any
	if err := common.Unmarshal(jsonData, &data); err != nil {
		return jsonData, false, err
	}

	inputAny, ok := data["input"]
	if !ok {
		return jsonData, false, nil
	}

	inputs, ok := inputAny.([]any)
	if !ok {
		return jsonData, false, nil
	}

	sanitizedInputs := make([]any, 0, len(inputs))
	modified := false

	for _, rawItem := range inputs {
		item, ok := rawItem.(map[string]any)
		if !ok {
			sanitizedInputs = append(sanitizedInputs, rawItem)
			continue
		}

		if _, exists := item["metadata"]; exists {
			delete(item, "metadata")
			modified = true
		}
		if _, exists := item["internal_chat_message_metadata_passthrough"]; exists {
			delete(item, "internal_chat_message_metadata_passthrough")
			modified = true
		}

		if stripEncrypted {
			if _, exists := item["encrypted_content"]; exists {
				modified = true
				continue
			}
			if itemType, _ := item["type"].(string); itemType == "encrypted_content" || itemType == "reasoning" {
				if _, exists := item["encrypted_content"]; exists {
					modified = true
					continue
				}
			}
		}

		if stripEncrypted {
			contentAny, hasContent := item["content"]
			if hasContent {
				contentItems, ok := contentAny.([]any)
				if ok {
					sanitizedContent := make([]any, 0, len(contentItems))
					for _, rawContent := range contentItems {
						contentMap, ok := rawContent.(map[string]any)
						if !ok {
							sanitizedContent = append(sanitizedContent, rawContent)
							continue
						}
						if _, exists := contentMap["encrypted_content"]; exists {
							modified = true
							continue
						}
						if contentType, _ := contentMap["type"].(string); contentType == "encrypted_content" {
							modified = true
							continue
						}
						sanitizedContent = append(sanitizedContent, contentMap)
					}
					if len(sanitizedContent) != len(contentItems) {
						item["content"] = sanitizedContent
						modified = true
					}
					if len(sanitizedContent) == 0 {
						if role, _ := item["role"].(string); role == "assistant" {
							modified = true
							continue
						}
					}
				}
			}
		}

		sanitizedInputs = append(sanitizedInputs, item)
	}

	if !modified {
		return jsonData, false, nil
	}

	data["input"] = sanitizedInputs
	out, err := common.Marshal(data)
	if err != nil {
		return jsonData, false, err
	}
	return out, true, nil
}
