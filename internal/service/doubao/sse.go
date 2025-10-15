package doubao

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"DoubaoProxy/internal/model"
)

type sseEnvelope struct {
	EventType int             `json:"event_type"`
	EventData json.RawMessage `json:"event_data"`
}

func parseSSE(r io.Reader) (text string, images []string, conversationID, messageID, sectionID string, err error) {
	reader := bufio.NewReader(r)
	var builder strings.Builder
	texts := make([]string, 0)
	images = make([]string, 0)

	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return "", nil, "", "", "", fmt.Errorf("read sse: %w", readErr)
		}

		if strings.Contains(line, "tourist conversation reach limited") {
			return "", nil, "", "", "", model.NewHTTPError(http.StatusTooManyRequests, "tourist session limit reached; please refresh session")
		}

		builder.WriteString(line)
		if strings.TrimSpace(line) != "" && !errors.Is(readErr, io.EOF) {
			if readErr != nil {
				break
			}
			continue
		}

		block := builder.String()
		builder.Reset()
		if strings.TrimSpace(block) == "" {
			if errors.Is(readErr, io.EOF) {
				break
			}
			if readErr != nil {
				break
			}
			continue
		}

		eventName, dataLine := parseEventBlock(block)
		if eventName == "gateway-error" {
			if msg := dataLine; msg != "" {
				return "", nil, "", "", "", model.NewHTTPError(http.StatusBadGateway, msg)
			}
			return "", nil, "", "", "", model.NewHTTPError(http.StatusBadGateway, "doubao gateway error")
		}

		if dataLine == "" {
			if errors.Is(readErr, io.EOF) {
				break
			}
			if readErr != nil {
				break
			}
			continue
		}

		var envelope sseEnvelope
		if err := json.Unmarshal([]byte(dataLine), &envelope); err != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			continue
		}

		data, err := decodeEventData(envelope.EventData)
		if err != nil {
			continue
		}

		switch envelope.EventType {
		case 2001:
			messageMap, _ := data["message"].(map[string]any)
			if len(messageMap) == 0 {
				continue
			}
			contentType := getInt(messageMap["content_type"])
			switch contentType {
			case 10000, 2001, 2008:
				if txt := extractText(messageMap["content"]); txt != "" {
					texts = append(texts, txt)
				}
			case 2074:
				images = appendUnique(images, extractImages(messageMap["content"])...)
			default:
				continue
			}
		case 2002:
			if v := getString(data["conversation_id"], ""); v != "" {
				conversationID = v
			}
			if v := getString(data["message_id"], ""); v != "" {
				messageID = v
			}
			if v := getString(data["section_id"], ""); v != "" {
				sectionID = v
			}
		case 2003:
			if v := getString(data["conversation_id"], conversationID); v != "" {
				conversationID = v
			}
			if v := getString(data["message_id"], messageID); v != "" {
				messageID = v
			}
			if v := getString(data["section_id"], sectionID); v != "" {
				sectionID = v
			}
			text = strings.Join(texts, "")
			return text, images, conversationID, messageID, sectionID, nil
		}

		if errors.Is(readErr, io.EOF) {
			break
		}
	}

	if len(texts) == 0 && len(images) == 0 {
		return "", nil, "", "", "", model.NewHTTPError(http.StatusBadGateway, "empty response from doubao")
	}

	text = strings.Join(texts, "")
	return text, images, conversationID, messageID, sectionID, nil
}

func parseEventBlock(block string) (eventName string, data string) {
	lines := strings.Split(block, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "event:"):
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
	}
	return eventName, data
}

func decodeEventData(raw json.RawMessage) (map[string]any, error) {
	result := make(map[string]any)
	if len(raw) == 0 {
		return result, nil
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		if strings.TrimSpace(asString) == "" {
			return result, nil
		}
		decoder := json.NewDecoder(strings.NewReader(asString))
		decoder.UseNumber()
		if err := decoder.Decode(&result); err != nil {
			return nil, err
		}
		return result, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func getString(value any, fallback string) string {
	switch v := value.(type) {
	case string:
		if v != "" {
			return v
		}
	case json.Number:
		if v != "" {
			return v.String()
		}
	}
	return fallback
}

func getInt(value any) int {
	switch v := value.(type) {
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

func extractText(value any) string {
	str, _ := value.(string)
	if strings.TrimSpace(str) == "" {
		return ""
	}
	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(str), &payload); err != nil {
		return ""
	}
	return payload.Text
}

func extractImages(value any) []string {
	str, _ := value.(string)
	if strings.TrimSpace(str) == "" {
		return nil
	}
	var payload struct {
		Creations []struct {
			Image struct {
				Status   int `json:"status"`
				ImageRaw struct {
					URL string `json:"url"`
				} `json:"image_raw"`
				ImageThumb struct {
					URL string `json:"url"`
				} `json:"image_thumb"`
				ImageOri struct {
					URL string `json:"url"`
				} `json:"image_ori"`
			} `json:"image"`
		} `json:"creations"`
	}
	if err := json.Unmarshal([]byte(str), &payload); err != nil {
		return nil
	}
	urls := make([]string, 0)
	for _, creation := range payload.Creations {
		if creation.Image.Status != 2 {
			continue
		}
		switch {
		case creation.Image.ImageRaw.URL != "":
			urls = append(urls, creation.Image.ImageRaw.URL)
		case creation.Image.ImageThumb.URL != "":
			urls = append(urls, creation.Image.ImageThumb.URL)
		case creation.Image.ImageOri.URL != "":
			urls = append(urls, creation.Image.ImageOri.URL)
		}
	}
	return urls
}

func appendUnique(dst []string, values ...string) []string {
	seen := make(map[string]struct{}, len(dst))
	for _, item := range dst {
		seen[item] = struct{}{}
	}
	for _, v := range values {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		dst = append(dst, v)
	}
	return dst
}
