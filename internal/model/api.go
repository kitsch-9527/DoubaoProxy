package model

// CompletionRequest 表示聊天补全接口的请求体。
type CompletionRequest struct {
	Prompt         string       `json:"prompt" binding:"required"`
	Guest          bool         `json:"guest"`
	Attachments    []Attachment `json:"attachments,omitempty"`
	ConversationID string       `json:"conversation_id,omitempty"`
	SectionID      string       `json:"section_id,omitempty"`
	UseDeepThink   bool         `json:"use_deep_think"`
	UseAutoCoT     bool         `json:"use_auto_cot"`
}

// Attachment 对应豆包 API 所要求的附件结构。
type Attachment struct {
	Key             string         `json:"key"`
	Name            string         `json:"name"`
	Type            string         `json:"type"`
	FileReviewState int            `json:"file_review_state"`
	FileParseState  int            `json:"file_parse_state"`
	Identifier      string         `json:"identifier"`
	Option          map[string]any `json:"option,omitempty"`
	MD5             string         `json:"md5,omitempty"`
	Size            int            `json:"size,omitempty"`
}

// CompletionResponse 描述返回给客户端的补全结果。
type CompletionResponse struct {
	Text           string   `json:"text"`
	ImgURLs        []string `json:"img_urls"`
	ConversationID string   `json:"conversation_id"`
	MessageID      string   `json:"messageg_id"`
	SectionID      string   `json:"section_id"`
}

// DeleteResponse 是删除会话接口的响应结构。
type DeleteResponse struct {
	OK  bool   `json:"ok"`
	Msg string `json:"msg"`
}

// UploadResponse 描述文件上传完成后返回的数据。
type UploadResponse struct {
	Key             string         `json:"key"`
	Name            string         `json:"name"`
	Type            string         `json:"type"`
	FileReviewState int            `json:"file_review_state"`
	FileParseState  int            `json:"file_parse_state"`
	Identifier      string         `json:"identifier"`
	Option          map[string]any `json:"option,omitempty"`
	MD5             string         `json:"md5,omitempty"`
	Size            int            `json:"size,omitempty"`
}
