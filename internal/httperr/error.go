package httperr

type Field struct {
	Path string `json:"path,omitempty"`
	Code string `json:"code"`
}

type Body struct {
	Error Error `json:"error"`
}

type Error struct {
	Code      string  `json:"code"`
	Message   string  `json:"message"`
	RequestID string  `json:"requestId,omitempty"`
	Details   []Field `json:"details,omitempty"`
}

func New(code, message, requestID string, details ...Field) Body {
	return Body{Error: Error{
		Code:      code,
		Message:   message,
		RequestID: requestID,
		Details:   details,
	}}
}
