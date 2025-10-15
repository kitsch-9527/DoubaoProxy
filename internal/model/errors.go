package model

import "fmt"

// HTTPError 表示携带 HTTP 状态码的业务错误。
type HTTPError struct {
	Status  int
	Message string
}

func (e *HTTPError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// StatusCode 返回对应的 HTTP 状态码。
func (e *HTTPError) StatusCode() int {
	if e == nil {
		return 0
	}
	return e.Status
}

// NewHTTPError 根据格式化字符串构造一个 HTTPError。
func NewHTTPError(status int, format string, args ...any) *HTTPError {
	return &HTTPError{
		Status:  status,
		Message: fmt.Sprintf(format, args...),
	}
}
