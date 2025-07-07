package dialog

import (
	"fmt"
	"strconv"
	"sync"
)

// SequenceManager управляет CSeq номерами для диалога
//
// RFC 3261 Section 8.1.1.5:
// - CSeq должен увеличиваться для каждого нового запроса в диалоге
// - CSeq состоит из числа и метода
// - ACK для non-2xx ответов использует тот же CSeq что и INVITE
// - ACK для 2xx ответов использует тот же номер но метод ACK
type SequenceManager struct {
	mu           sync.Mutex
	localCSeq    uint32 // Текущий локальный CSeq
	remoteCSeq   uint32 // Последний принятый удаленный CSeq
	isUAC        bool   // Роль в диалоге
	inviteCSeq   uint32 // CSeq от INVITE (для ACK)
	inviteMethod string // Метод INVITE (обычно "INVITE")
}

// NewSequenceManager создает новый менеджер CSeq
//
// Параметры:
//   - initialLocal: начальный локальный CSeq (обычно случайное число)
//   - isUAC: true если этот UA инициировал диалог
func NewSequenceManager(initialLocal uint32, isUAC bool) *SequenceManager {
	return &SequenceManager{
		localCSeq:  initialLocal,
		remoteCSeq: 0,
		isUAC:      isUAC,
	}
}

// NextLocalCSeq возвращает следующий локальный CSeq для нового запроса
//
// RFC 3261: CSeq должен строго увеличиваться для каждого нового запроса
func (sm *SequenceManager) NextLocalCSeq() uint32 {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	sm.localCSeq++
	return sm.localCSeq
}

// GetLocalCSeq возвращает текущий локальный CSeq без инкремента
func (sm *SequenceManager) GetLocalCSeq() uint32 {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	return sm.localCSeq
}

// ValidateRemoteCSeq проверяет входящий CSeq от удаленной стороны
//
// RFC 3261 Section 12.2.2:
// - CSeq должен строго увеличиваться
// - Исключение: ретрансмиссии и ACK
//
// Возвращает true если CSeq валиден
func (sm *SequenceManager) ValidateRemoteCSeq(cseq uint32, method string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	// Первый запрос от удаленной стороны
	if sm.remoteCSeq == 0 {
		sm.remoteCSeq = cseq
		return true
	}
	
	// ACK может иметь тот же CSeq что и INVITE
	if method == "ACK" {
		return cseq == sm.inviteCSeq || cseq == sm.remoteCSeq
	}
	
	// Ретрансмиссия (тот же CSeq)
	if cseq == sm.remoteCSeq {
		return true
	}
	
	// Новый запрос должен иметь больший CSeq
	if cseq > sm.remoteCSeq {
		sm.remoteCSeq = cseq
		return true
	}
	
	return false
}

// SetInviteCSeq сохраняет CSeq от INVITE для последующих ACK
func (sm *SequenceManager) SetInviteCSeq(cseq uint32, method string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	if method == "INVITE" {
		sm.inviteCSeq = cseq
		sm.inviteMethod = method
	}
}

// GetInviteCSeq возвращает сохраненный CSeq от INVITE
func (sm *SequenceManager) GetInviteCSeq() uint32 {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	return sm.inviteCSeq
}

// ParseCSeq извлекает номер и метод из заголовка CSeq
//
// Формат: "number method" например "1 INVITE"
func ParseCSeq(cseqHeader string) (uint32, string, error) {
	// Находим первый пробел
	spaceIdx := -1
	for i, ch := range cseqHeader {
		if ch == ' ' || ch == '\t' {
			spaceIdx = i
			break
		}
	}
	
	if spaceIdx == -1 {
		return 0, "", fmt.Errorf("invalid CSeq format: %s", cseqHeader)
	}
	
	// Парсим номер
	numStr := cseqHeader[:spaceIdx]
	num, err := strconv.ParseUint(numStr, 10, 32)
	if err != nil {
		return 0, "", fmt.Errorf("invalid CSeq number: %s", numStr)
	}
	
	// Извлекаем метод (пропускаем пробелы)
	methodStart := spaceIdx + 1
	for methodStart < len(cseqHeader) && (cseqHeader[methodStart] == ' ' || cseqHeader[methodStart] == '\t') {
		methodStart++
	}
	
	if methodStart >= len(cseqHeader) {
		return 0, "", fmt.Errorf("missing method in CSeq: %s", cseqHeader)
	}
	
	method := cseqHeader[methodStart:]
	
	// Обрезаем конечные пробелы в методе
	methodEnd := len(method)
	for methodEnd > 0 && (method[methodEnd-1] == ' ' || method[methodEnd-1] == '\t') {
		methodEnd--
	}
	method = method[:methodEnd]
	
	return uint32(num), method, nil
}

// FormatCSeq форматирует CSeq заголовок
func FormatCSeq(cseq uint32, method string) string {
	return fmt.Sprintf("%d %s", cseq, method)
}

// GenerateInitialCSeq генерирует начальный CSeq номер
//
// RFC 3261 рекомендует использовать случайное начальное значение
func GenerateInitialCSeq() uint32 {
	// Простая реализация
	// TODO: использовать crypto/rand в production
	return uint32(timeNow().UnixNano() % 2147483647) // Max 31-bit
}