package dialog

type Body struct {
	contentType string
	content     []byte
}

func (b *Body) Type() string {
	return b.contentType
}

func (b *Body) Content() []byte {
	return b.content
}

func NewBody(contentType string, content []byte) Body {
	return Body{contentType, content}
}

func (b *Body) Copy() *Body {
	var bodyCopy []byte
	copy(bodyCopy, b.content)

	return &Body{b.contentType, bodyCopy}
}
