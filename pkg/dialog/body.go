package dialog

// BodyImpl представляет стандартную реализацию интерфейса Body.
//
// Обеспечивает базовую функциональность для хранения и передачи тела SIP сообщений.
// Поддерживает любой MIME тип и бинарные данные.
type BodyImpl struct {
	// contentType MIME тип содержимого (например, "application/sdp")
	contentType string
	// content байты содержимого
	content []byte
}

// ContentType возвращает MIME тип содержимого.
// Реализует метод интерфейса Body.
func (b *BodyImpl) ContentType() string {
	return b.contentType
}

// Data возвращает байты содержимого.
// Реализует метод интерфейса Body.
func (b *BodyImpl) Data() []byte {
	return b.content
}

// NewBody создает новый экземпляр BodyImpl с указанным содержимым.
//
// Параметры:
//   - contentType: MIME тип (например, "application/sdp", "text/plain")
//   - content: байты содержимого
//
// Пример использования:
//
//	sdpBody := NewBody("application/sdp", []byte("v=0..."))
//	textBody := NewBody("text/plain", []byte("Привет мир!"))
func NewBody(contentType string, content []byte) *BodyImpl {
	return &BodyImpl{contentType, content}
}

// Copy создает независимую копию тела сообщения.
//
// Копирует как тип содержимого, так и байты данных.
// Используется для безопасного разделения данных.
func (b *BodyImpl) Copy() *BodyImpl {
	// Правильное копирование слайса
	bodyCopy := make([]byte, len(b.content))
	copy(bodyCopy, b.content)

	return &BodyImpl{b.contentType, bodyCopy}
}
