package dialog

import (
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
)

// GenerateDialogKey создает ключ диалога из SIP сообщения с учетом роли UAC/UAS
//
// RFC 3261 Section 12: Диалог идентифицируется тремя компонентами:
//   - Call-ID
//   - From tag (local tag для UAC, remote tag для UAS)
//   - To tag (remote tag для UAC, local tag для UAS)
//
// Параметры:
//   - msg: SIP сообщение (Request или Response)
//   - isUAS: true если этот UA выступает в роли UAS (получатель INVITE)
//
// Возвращает:
//   - DialogKey с правильной ориентацией тегов
//   - error если сообщение не содержит необходимых заголовков
func GenerateDialogKey(msg types.Message, isUAS bool) (DialogKey, error) {
	// Извлекаем Call-ID
	callID := msg.GetHeader("Call-ID")
	if callID == "" {
		return DialogKey{}, &DialogError{
			Code:    400,
			Message: "Missing Call-ID header",
		}
	}

	// Извлекаем From tag
	fromHeader := msg.GetHeader("From")
	if fromHeader == "" {
		return DialogKey{}, &DialogError{
			Code:    400,
			Message: "Missing From header",
		}
	}
	fromTag := extractTag(fromHeader)
	if fromTag == "" {
		return DialogKey{}, &DialogError{
			Code:    400,
			Message: "Missing From tag",
		}
	}

	// Извлекаем To tag (может отсутствовать в начальных запросах)
	toHeader := msg.GetHeader("To")
	if toHeader == "" {
		return DialogKey{}, &DialogError{
			Code:    400,
			Message: "Missing To header",
		}
	}
	toTag := extractTag(toHeader)

	// Определяем local и remote tags в зависимости от роли
	var localTag, remoteTag string
	if isUAS {
		// Для UAS: local = To tag, remote = From tag
		localTag = toTag
		remoteTag = fromTag
	} else {
		// Для UAC: local = From tag, remote = To tag
		localTag = fromTag
		remoteTag = toTag
	}

	return DialogKey{
		CallID:    callID,
		LocalTag:  localTag,
		RemoteTag: remoteTag,
	}, nil
}

// GenerateLocalTag генерирует уникальный локальный тег для диалога
//
// RFC 3261 рекомендует использовать криптографически случайные теги
// для обеспечения уникальности и безопасности
func GenerateLocalTag() string {
	// Используем простую реализацию для начала
	// В production следует использовать crypto/rand
	return generateRandomString(8)
}

// extractTag извлекает значение параметра tag из заголовка From/To
//
// Формат: "Display Name" <sip:user@host>;tag=value
func extractTag(header string) string {
	// Ищем параметр tag
	tagIndex := findParameter(header, "tag")
	if tagIndex == -1 {
		return ""
	}

	// Извлекаем значение после "tag="
	start := tagIndex + 4 // len("tag=")
	end := start
	for end < len(header) && header[end] != ';' && header[end] != ' ' {
		end++
	}

	return header[start:end]
}

// findParameter находит позицию параметра в заголовке
func findParameter(header, param string) int {
	paramWithEquals := param + "="
	idx := 0
	for idx < len(header) {
		pos := findSubstring(header[idx:], paramWithEquals)
		if pos == -1 {
			return -1
		}
		idx += pos
		
		// Проверяем, что это начало параметра (после ; или начало строки)
		if idx == 0 || header[idx-1] == ';' || header[idx-1] == ' ' {
			return idx
		}
		idx++
	}
	return -1
}

// findSubstring простой поиск подстроки
func findSubstring(s, substr string) int {
	if len(substr) > len(s) {
		return -1
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// generateRandomString генерирует случайную строку заданной длины
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	
	// Простая реализация для начала
	// TODO: использовать crypto/rand в production
	for i := range result {
		// Используем текущее время и индекс для псевдослучайности
		result[i] = charset[(i*17+int(timeNow().UnixNano()))%len(charset)]
	}
	
	return string(result)
}

// timeNow обертка для тестирования
var timeNow = func() time.Time {
	return time.Now()
}

// DialogError представляет ошибку диалога с SIP кодом
type DialogError struct {
	Code    int
	Message string
}

func (e *DialogError) Error() string {
	return e.Message
}