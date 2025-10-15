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

	"DoubaoProxy/internal/model"
	"DoubaoProxy/internal/session"
)

// DeleteConversation 调用豆包接口删除指定的会话。
func (s *Service) DeleteConversation(ctx context.Context, conversationID string) (*model.DeleteResponse, error) {
	session, err := s.pool.GetSession(conversationID, false)
	if err != nil {
		return nil, err
	}

	endpoint := buildDeleteURL(session)
	body := map[string]string{"conversation_id": conversationID}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal delete payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create delete request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", session.Cookie)
	req.Header.Set("Origin", "https://www.doubao.com")
	req.Header.Set("Referer", fmt.Sprintf("https://www.doubao.com/chat/%s", conversationID))
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call doubao delete: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return &model.DeleteResponse{OK: false, Msg: strings.TrimSpace(string(bodyBytes))}, nil
	}

	s.pool.ForgetConversation(conversationID)
	return &model.DeleteResponse{OK: true, Msg: ""}, nil
}

func buildDeleteURL(session *session.Session) string {
	values := url.Values{}
	values.Set("aid", "497858")
	values.Set("device_id", session.DeviceID)
	values.Set("device_platform", "web")
	values.Set("language", "zh")
	values.Set("pc_version", "2.20.0")
	values.Set("pkg_type", "release_version")
	values.Set("real_aid", "497858")
	values.Set("region", "CN")
	values.Set("samantha_web", "1")
	values.Set("sys_region", "CN")
	values.Set("tea_uuid", session.TeaUUID)
	values.Set("use-olympus-account", "1")
	values.Set("version_code", "20800")
	values.Set("web_id", session.WebID)
	return "https://www.doubao.com/samantha/thread/delete?" + values.Encode()
}
