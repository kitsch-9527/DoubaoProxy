package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"sync"
	"time"

	"DoubaoProxy/internal/model"
)

// Session 保存调用豆包接口所需的一组凭证。
type Session struct {
	Cookie     string `json:"cookie"`
	DeviceID   string `json:"device_id"`
	TeaUUID    string `json:"tea_uuid"`
	WebID      string `json:"web_id"`
	RoomID     string `json:"room_id"`
	XFlowTrace string `json:"x_flow_trace"`
	Guest      bool   `json:"guest,omitempty"`
}

func (s *Session) validate() error {
	switch {
	case s == nil:
		return errors.New("nil session")
	case s.Cookie == "":
		return errors.New("cookie is required")
	case s.DeviceID == "":
		return errors.New("device_id is required")
	case s.TeaUUID == "":
		return errors.New("tea_uuid is required")
	case s.WebID == "":
		return errors.New("web_id is required")
	case s.RoomID == "":
		return errors.New("room_id is required")
	case s.XFlowTrace == "":
		return errors.New("x_flow_trace is required")
	default:
		return nil
	}
}

// Pool 负责管理所有 Session 并维护与会话 ID 的关联关系。
type Pool struct {
	mu              sync.RWMutex
	configPath      string
	conversationMap map[string]*Session
	authSessions    []*Session
	guestSessions   []*Session
	rng             *rand.Rand
}

// NewPool 根据配置文件初始化会话池。
func NewPool(configPath string) (*Pool, error) {
	p := &Pool{
		configPath:      configPath,
		conversationMap: make(map[string]*Session),
		rng:             rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	if err := p.loadFromFile(); err != nil {
		return nil, err
	}
	return p, nil
}

// GetSession 返回指定会话 ID 对应的 Session，若未找到则按类型随机挑选一份。
func (p *Pool) GetSession(conversationID string, guest bool) (*Session, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if conversationID != "" {
		if session, ok := p.conversationMap[conversationID]; ok {
			return session, nil
		}
	}

	sessions := p.authSessions
	if guest {
		sessions = p.guestSessions
	}
	if len(sessions) == 0 {
		if guest {
			return nil, model.NewHTTPError(404, "no guest sessions configured")
		}
		return nil, model.NewHTTPError(404, "no authenticated sessions configured")
	}
	session := sessions[p.rng.Intn(len(sessions))]
	return session, nil
}

// BindConversation 将会话 ID 与具体 Session 绑定。
func (p *Pool) BindConversation(conversationID string, s *Session) {
	if conversationID == "" || s == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.conversationMap[conversationID] = s
}

// ForgetConversation 移除会话 ID 与 Session 的映射。
func (p *Pool) ForgetConversation(conversationID string) {
	if conversationID == "" {
		return
	}
	p.mu.Lock()
	delete(p.conversationMap, conversationID)
	p.mu.Unlock()
}

// RemoveSession 将失效的 Session 从池中剔除（例如豆包封禁该凭证时）。
func (p *Pool) RemoveSession(target *Session) {
	if target == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	slice := &p.authSessions
	if target.Guest {
		slice = &p.guestSessions
	}
	filtered := (*slice)[:0]
	for _, s := range *slice {
		if s != target {
			filtered = append(filtered, s)
		}
	}
	*slice = filtered
}

func (p *Pool) loadFromFile() error {
	file, err := os.Open(p.configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			slog.Warn("session config file not found", "path", p.configPath)
			return nil
		}
		return fmt.Errorf("open session config: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()

	var entries []Session
	if err := decoder.Decode(&entries); err != nil {
		return fmt.Errorf("decode session config: %w", err)
	}

	for _, entry := range entries {
		entry := entry
		if err := entry.validate(); err != nil {
			slog.Warn("skip invalid session", "error", err)
			continue
		}
		if entry.Guest {
			p.guestSessions = append(p.guestSessions, &entry)
		} else {
			p.authSessions = append(p.authSessions, &entry)
		}
	}

	if len(p.authSessions) == 0 && len(p.guestSessions) == 0 {
		slog.Warn("session pool is empty", "path", p.configPath)
	}
	return nil
}
