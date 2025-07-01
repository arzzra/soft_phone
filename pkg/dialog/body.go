package dialog

type BodyImpl struct {
	contentType string
	content     []byte
}

func (b *BodyImpl) ContentType() string {
	return b.contentType
}

func (b *BodyImpl) Data() []byte {
	return b.content
}

func NewBody(contentType string, content []byte) *BodyImpl {
	return &BodyImpl{contentType, content}
}

func (b *BodyImpl) Copy() *BodyImpl {
	var bodyCopy []byte
	copy(bodyCopy, b.content)

	return &BodyImpl{b.contentType, bodyCopy}
}
