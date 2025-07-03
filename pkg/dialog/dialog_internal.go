package dialog

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/emiago/sipgo/sip"
)

// generateBranch генерирует уникальный branch для Via заголовка
func generateBranch() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback to pseudorandom if crypto/rand fails
		for i := range b {
			b[i] = byte(time.Now().UnixNano() + int64(i))
		}
	}
	return "z9hG4bK" + hex.EncodeToString(b)
}

// ВАЖНО: generateCallID() и generateTag() перенесены в id_generator.go
// для оптимизированной thread-safe генерации с пулированием

// incrementCSeq увеличивает локальный CSeq для нового запроса
func (d *Dialog) incrementCSeq() uint32 {
	return atomic.AddUint32(&d.localSeq, 1)
}

// buildRequest создает новый запрос в контексте диалога
func (d *Dialog) buildRequest(method sip.RequestMethod) (*sip.Request, error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	// КРИТИЧНО: Диагностика для отладки проблемы с тегами
	if d.stack != nil && d.stack.config.Logger != nil && method == sip.BYE {
		d.stack.config.Logger.Printf("Building %s request for dialog %s: isUAC=%v, localTag=%s, remoteTag=%s", 
			method, d.callID, d.isUAC, d.localTag, d.remoteTag)
	}

	// Определяем Request-URI
	reqURI := d.remoteTarget
	if reqURI.Host == "" {
		// Если нет remote target, используем исходный URI
		if d.inviteReq != nil {
			reqURI = d.inviteReq.Recipient
		} else {
			return nil, fmt.Errorf("no remote target for request")
		}
	}

	// Создаем базовый запрос
	req := sip.NewRequest(method, reqURI)

	// Устанавливаем Call-ID
	req.AppendHeader(sip.NewHeader("Call-ID", d.callID))

	// From и To зависят от роли (UAC/UAS)
	var fromTag, toTag string
	var fromURI, toURI sip.Uri

	if d.isUAC {
		fromTag = d.localTag
		toTag = d.remoteTag
		if d.inviteReq != nil {
			fromURI = d.inviteReq.From().Address
			toURI = d.inviteReq.To().Address
		}
	} else {
		fromTag = d.remoteTag
		toTag = d.localTag
		if d.inviteReq != nil {
			fromURI = d.inviteReq.To().Address
			toURI = d.inviteReq.From().Address
		}
	}

	// КРИТИЧНО: Дополнительная диагностика для BYE запросов
	if d.stack != nil && d.stack.config.Logger != nil && method == sip.BYE {
		d.stack.config.Logger.Printf("BYE request tags: From-tag=%s, To-tag=%s", fromTag, toTag)
		if fromTag == toTag {
			d.stack.config.Logger.Printf("WARNING: From and To tags are identical! This will cause 481 errors.")
		}
	}

	// From header
	fromHeader := &sip.FromHeader{
		DisplayName: "",
		Address:     fromURI,
		Params:      sip.HeaderParams{"tag": fromTag},
	}
	req.AppendHeader(fromHeader)

	// To header
	toHeader := &sip.ToHeader{
		DisplayName: "",
		Address:     toURI,
		Params:      sip.HeaderParams{},
	}
	if toTag != "" {
		toHeader.Params["tag"] = toTag
	}
	req.AppendHeader(toHeader)

	// CSeq
	cseq := d.incrementCSeq()
	req.AppendHeader(&sip.CSeqHeader{
		SeqNo:      cseq,
		MethodName: method,
	})

	// Max-Forwards
	req.AppendHeader(sip.NewHeader("Max-Forwards", "70"))

	// Via будет добавлен транспортным уровнем

	// Contact
	if d.localContact.Address.Host != "" {
		req.AppendHeader(&d.localContact)
	}

	// Route headers (если есть)
	for _, route := range d.routeSet {
		req.AppendHeader(&sip.RouteHeader{Address: route})
	}

	// User-Agent
	if d.stack != nil && d.stack.config.UserAgent != "" {
		req.AppendHeader(sip.NewHeader("User-Agent", d.stack.config.UserAgent))
	}

	return req, nil
}

// processResponse обрабатывает ответ и обновляет состояние диалога
func (d *Dialog) processResponse(resp *sip.Response) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	// КРИТИЧНО: Сохраняем оригинальный localTag для валидации
	originalLocalTag := d.localTag

	// КРИТИЧНО: Диагностика processResponse
	if d.stack != nil && d.stack.config.Logger != nil {
		d.stack.config.Logger.Printf("Processing response %d for dialog %s: before update localTag=%s, remoteTag=%s (instance=%p)", 
			resp.StatusCode, d.callID, d.localTag, d.remoteTag, d)
	}

	// КРИТИЧНО: Обновляем remote tag и ключ диалога thread-safe
	needsKeyUpdate := false
	var oldKey DialogKey
	
	if d.remoteTag == "" {
		// Сохраняем старый ключ
		oldKey = d.key
		
		if d.isUAC {
			// Для UAC remote tag приходит в To заголовке ответа
			if toTag := resp.To().Params["tag"]; toTag != "" {
				// КРИТИЧНО: Валидация что теги разные
				if toTag == d.localTag {
					if d.stack != nil && d.stack.config.Logger != nil {
						d.stack.config.Logger.Printf("ERROR: Remote tag equals local tag (%s) for UAC dialog %s", toTag, d.callID)
					}
					return fmt.Errorf("remote tag cannot be the same as local tag")
				}
				d.remoteTag = toTag
				d.key.RemoteTag = toTag
				needsKeyUpdate = true
			}
		} else {
			// Для UAS remote tag должен быть уже установлен при создании диалога
			// Это ситуация когда сервер отправляет ответ и получает на него ACK/другой запрос
			if fromTag := resp.From().Params["tag"]; fromTag != "" && d.remoteTag == "" {
				// КРИТИЧНО: Валидация что теги разные
				if fromTag == d.localTag {
					if d.stack != nil && d.stack.config.Logger != nil {
						d.stack.config.Logger.Printf("ERROR: Remote tag equals local tag (%s) for UAS dialog %s", fromTag, d.callID)
					}
					return fmt.Errorf("remote tag cannot be the same as local tag")
				}
				d.remoteTag = fromTag
				d.key.RemoteTag = fromTag
				needsKeyUpdate = true
			}
		}
	}
	
	// КРИТИЧНО: Диагностика после обновления тегов
	if d.stack != nil && d.stack.config.Logger != nil && needsKeyUpdate {
		d.stack.config.Logger.Printf("Updated dialog tags for %s: localTag=%s, remoteTag=%s, newKey=%s", 
			d.callID, d.localTag, d.remoteTag, d.key.String())
	}
	
	// КРИТИЧНО: Атомарно обновляем диалог в карте стека
	if needsKeyUpdate && d.stack != nil && d.stack.dialogs != nil {
		// ВАЖНО: Делаем это под mutex'ом диалога для предотвращения race conditions
		// Удаляем под старым ключом если он отличается
		if oldKey.RemoteTag != d.key.RemoteTag {
			// НОВОЕ: Проверяем что диалог еще существует под старым ключом
			if existingDialog, exists := d.stack.findDialogByKey(oldKey); exists && existingDialog == d {
				d.stack.removeDialog(oldKey)
				// Добавляем под новым ключом
				d.stack.addDialog(d.key, d)
			}
		}
	}

	// Обновляем remote target из Contact (для 2xx ответов)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if contact := resp.GetHeader("Contact"); contact != nil {
			// Парсим Contact URI
			contactStr := contact.Value()
			if strings.HasPrefix(contactStr, "<") && strings.HasSuffix(contactStr, ">") {
				contactStr = strings.TrimPrefix(contactStr, "<")
				contactStr = strings.TrimSuffix(contactStr, ">")
			}
			// Используем правильный парсинг Contact URI
			var contactUri sip.Uri
			if err := sip.ParseUri(contactStr, &contactUri); err != nil {
				// Если парсинг не удался, логируем ошибку и пропускаем
				if d.stack != nil && d.stack.config.Logger != nil {
					d.stack.config.Logger.Printf("Failed to parse Contact URI: %v", err)
				}
			} else {
				d.remoteTarget = contactUri
			}
		}

		// Обновляем route set из Record-Route (в обратном порядке для UAC)
		d.routeSet = nil
		recordRoutes := resp.GetHeaders("Record-Route")
		if d.isUAC {
			// UAC использует Record-Route в обратном порядке
			for i := len(recordRoutes) - 1; i >= 0; i-- {
				// Парсим Record-Route URI
				rrValue := recordRoutes[i].Value()
				// Удаляем угловые скобки, если есть
				if strings.HasPrefix(rrValue, "<") && strings.HasSuffix(rrValue, ">") {
					rrValue = strings.TrimPrefix(rrValue, "<")
					rrValue = strings.TrimSuffix(rrValue, ">")
				}

				var routeUri sip.Uri
				if err := sip.ParseUri(rrValue, &routeUri); err != nil {
					// Логируем ошибку парсинга
					if d.stack != nil && d.stack.config.Logger != nil {
						d.stack.config.Logger.Printf("Failed to parse Record-Route URI: %v", err)
					}
					continue
				}
				d.routeSet = append(d.routeSet, routeUri)
			}
		} else {
			// UAS использует Record-Route в прямом порядке
			for _, rr := range recordRoutes {
				// Парсим Record-Route URI
				rrValue := rr.Value()
				// Удаляем угловые скобки, если есть
				if strings.HasPrefix(rrValue, "<") && strings.HasSuffix(rrValue, ">") {
					rrValue = strings.TrimPrefix(rrValue, "<")
					rrValue = strings.TrimSuffix(rrValue, ">")
				}

				var routeUri sip.Uri
				if err := sip.ParseUri(rrValue, &routeUri); err != nil {
					// Логируем ошибку парсинга
					if d.stack != nil && d.stack.config.Logger != nil {
						d.stack.config.Logger.Printf("Failed to parse Record-Route URI: %v", err)
					}
					continue
				}
				d.routeSet = append(d.routeSet, routeUri)
			}
		}
	}

	// Сохраняем финальный ответ на INVITE
	if resp.StatusCode >= 200 && d.inviteResp == nil {
		d.inviteResp = resp
	}

	// КРИТИЧНО: Валидация что localTag не изменился
	if d.localTag != originalLocalTag {
		if d.stack != nil && d.stack.config.Logger != nil {
			d.stack.config.Logger.Printf("FATAL ERROR: localTag was changed in processResponse! Original=%s, Current=%s, Dialog=%s", 
				originalLocalTag, d.localTag, d.callID)
		}
		// Восстанавливаем оригинальный tag
		d.localTag = originalLocalTag
		return fmt.Errorf("internal error: localTag was corrupted during response processing")
	}

	return nil
}

// createResponse создает ответ на запрос в контексте диалога
func (d *Dialog) createResponse(req *sip.Request, statusCode int, reason string) *sip.Response {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	resp := sip.NewResponseFromRequest(req, statusCode, reason, nil)

	// Добавляем локальный tag в To для UAS
	if !d.isUAC && d.localTag != "" {
		to := resp.To()
		if to.Params == nil {
			to.Params = make(sip.HeaderParams)
		}
		to.Params["tag"] = d.localTag
		
		// КРИТИЧНО: Диагностика для отладки серверных тегов
		if d.stack != nil && d.stack.config.Logger != nil && statusCode >= 200 {
			d.stack.config.Logger.Printf("UAS created %d response for dialog %s: adding To-tag=%s", 
				statusCode, d.callID, d.localTag)
		}
	}

	// Contact
	if d.localContact.Address.Host != "" && statusCode >= 200 && statusCode < 300 {
		resp.AppendHeader(&d.localContact)
	}

	// Record-Route (копируем из запроса для 2xx)
	if statusCode >= 200 && statusCode < 300 {
		for _, rr := range req.GetHeaders("Record-Route") {
			resp.AppendHeader(rr)
		}
	}

	return resp
}

// buildACK создает ACK запрос для 2xx ответа на INVITE
func (d *Dialog) buildACK() (*sip.Request, error) {
	if d.inviteReq == nil || d.inviteResp == nil {
		return nil, fmt.Errorf("no INVITE transaction to ACK")
	}

	// ACK использует тот же Request-URI что и исходный INVITE
	ack := sip.NewRequest(sip.ACK, d.inviteReq.Recipient)

	// Call-ID как в INVITE
	ack.AppendHeader(sip.NewHeader("Call-ID", d.callID))

	// From как в INVITE (с tag)
	ack.AppendHeader(d.inviteReq.From())

	// To из ответа (с remote tag)
	ack.AppendHeader(d.inviteResp.To())

	// CSeq с тем же номером что и INVITE, но метод ACK
	inviteCSeq := d.inviteReq.CSeq()
	ack.AppendHeader(&sip.CSeqHeader{
		SeqNo:      inviteCSeq.SeqNo,
		MethodName: sip.ACK,
	})

	// Max-Forwards
	ack.AppendHeader(sip.NewHeader("Max-Forwards", "70"))

	// Route headers из route set
	for _, route := range d.routeSet {
		ack.AppendHeader(&sip.RouteHeader{Address: route})
	}

	return ack, nil
}

// matchesDialog проверяет, относится ли запрос/ответ к этому диалогу
func (d *Dialog) matchesDialog(callID string, fromTag string, toTag string) bool {
	if d.callID != callID {
		return false
	}

	// Для установленного диалога проверяем оба тега
	if d.localTag != "" && d.remoteTag != "" {
		if d.isUAC {
			return d.localTag == fromTag && d.remoteTag == toTag
		} else {
			return d.localTag == toTag && d.remoteTag == fromTag
		}
	}

	// Для early диалога (только local tag)
	if d.isUAC {
		return d.localTag == fromTag
	} else {
		return d.localTag == toTag
	}
}
