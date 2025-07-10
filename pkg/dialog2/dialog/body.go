package dialog

type Body struct {
	contentType string
	content     []byte
}

func (b *Body) ContentType() string {
	return b.contentType
}

func (b *Body) Content() []byte {
	return b.content
}

func (b *Body) SetContentType(contentType string) {
	b.contentType = contentType
}

func (b *Body) SetContent(content []byte) {
	b.content = content
}
