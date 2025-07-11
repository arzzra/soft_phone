package dialog

// Body представляет тело SIP сообщения.
// Может содержать SDP, XML или другие данные в зависимости от типа контента.
// Наиболее часто используется для передачи SDP (Session Description Protocol).
type Body struct {
	contentType string
	content     []byte
}

// ContentType возвращает тип содержимого (MIME type).
// Например: "application/sdp", "application/xml", "text/plain".
func (b *Body) ContentType() string {
	return b.contentType
}

// Content возвращает содержимое тела сообщения в виде байтов.
func (b *Body) Content() []byte {
	return b.content
}

// SetContentType устанавливает тип содержимого (MIME type).
func (b *Body) SetContentType(contentType string) {
	b.contentType = contentType
}

// SetContent устанавливает содержимое тела сообщения.
func (b *Body) SetContent(content []byte) {
	b.content = content
}
