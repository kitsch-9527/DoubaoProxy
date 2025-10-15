package doubao

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"DoubaoProxy/internal/model"
	"DoubaoProxy/internal/session"
)

// ChatCompletion 代理豆包的 SSE 聊天接口。
func (s *Service) ChatCompletion(ctx context.Context, req model.CompletionRequest) (*model.CompletionResponse, error) {
	session, err := s.pool.GetSession(req.ConversationID, req.Guest)
	if err != nil {
		return nil, err
	}

	endpoint := buildChatURL(session)
	body := buildChatPayload(req, session)
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal chat payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create chat request: %w", err)
	}

	// 豆包要求与浏览器一致的 SSE 请求头。
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Agw-Js-Conv", "str")
	httpReq.Header.Set("Cookie", session.Cookie)
	httpReq.Header.Set("Origin", "https://www.doubao.com")
	httpReq.Header.Set("Referer", fmt.Sprintf("https://www.doubao.com/chat/%s", session.RoomID))
	httpReq.Header.Set("User-Agent", defaultUserAgent)
	httpReq.Header.Set("X-Flow-Trace", session.XFlowTrace)

	resp, err := s.streamingClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call doubao chat: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, model.NewHTTPError(resp.StatusCode, "doubao chat failed: %s", strings.TrimSpace(string(bodyBytes)))
	}

	text, images, conversationID, messageID, sectionID, err := parseSSE(resp.Body)
	if err != nil {
		if httpErr, ok := err.(*model.HTTPError); ok && httpErr.StatusCode() == http.StatusTooManyRequests {
			s.pool.RemoveSession(session)
		}
		return nil, err
	}

	if conversationID != "" {
		s.pool.BindConversation(conversationID, session)
	}

	return &model.CompletionResponse{
		Text:           strings.TrimSpace(text),
		ImgURLs:        images,
		ConversationID: conversationID,
		MessageID:      messageID,
		SectionID:      sectionID,
	}, nil
}

func buildChatURL(session *session.Session) string {
	query := url.Values{}
	query.Set("aid", "497858")
	query.Set("device_id", session.DeviceID)
	query.Set("device_platform", "web")
	query.Set("language", "zh")
	query.Set("pc_version", "2.23.2")
	query.Set("pkg_type", "release_version")
	query.Set("real_aid", "497858")
	query.Set("region", "CN")
	query.Set("samantha_web", "1")
	query.Set("sys_region", "CN")
	query.Set("tea_uuid", session.TeaUUID)
	query.Set("use-olympus-account", "1")
	query.Set("version_code", "20800")
	query.Set("web_id", session.WebID)
	return "https://www.doubao.com/samantha/chat/completion?" + query.Encode()
}

func buildChatPayload(req model.CompletionRequest, session *session.Session) map[string]any {
	attachments := make([]map[string]any, 0, len(req.Attachments))
	for _, att := range req.Attachments {
		attachments = append(attachments, map[string]any{
			"key":               att.Key,
			"name":              att.Name,
			"type":              att.Type,
			"file_review_state": att.FileReviewState,
			"file_parse_state":  att.FileParseState,
			"identifier":        att.Identifier,
			"option":            att.Option,
			"md5":               att.MD5,
			"size":              att.Size,
		})
	}

	conversationID := req.ConversationID
	needCreate := conversationID == ""
	if needCreate {
		conversationID = "0"
	}

	payload := map[string]any{
		"completion_option": map[string]any{
			"is_regen":                 false,
			"with_suggest":             false,
			"need_create_conversation": needCreate,
			"launch_stage":             1,
			"use_auto_cot":             req.UseAutoCoT,
			"use_deep_think":           req.UseDeepThink,
		},
		"conversation_id": conversationID,
		"messages": []map[string]any{
			{
				"role":         0,
				"content":      mustJSON(map[string]string{"text": req.Prompt}),
				"content_type": 2001,
				"attachments":  attachments,
				"references":   []any{},
			},
		},
	}

	if req.SectionID != "" {
		payload["section_id"] = req.SectionID
	}

	if !session.Guest {
		payload["local_conversation_id"] = fmt.Sprintf("local_%d", time.Now().UnixNano()%1_0000_0000_0000_0000)
		payload["local_message_id"] = uuid.NewString()
	}

	return payload
}

func mustJSON(v any) string {
	buf, err := json.Marshal(v)
	if err != nil {
		// 正常情况下不会出错，如出错则回退为空对象。
		return "{}"
	}
	return string(buf)
}

const defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36 Edg/137.0.0.0"
