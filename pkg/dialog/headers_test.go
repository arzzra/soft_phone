package dialog

import (
	"strings"
	"testing"

	"github.com/emiago/sipgo/sip"
)

// TestContactURIParsing тестирует парсинг Contact заголовка
func TestContactURIParsing(t *testing.T) {
	tests := []struct {
		name         string
		contactValue string
		expectedUser string
		expectedHost string
		expectedPort int
		shouldFail   bool
	}{
		{
			name:         "Simple Contact",
			contactValue: "sip:alice@192.168.1.100:5060",
			expectedUser: "alice",
			expectedHost: "192.168.1.100",
			expectedPort: 5060,
		},
		{
			name:         "Contact with angle brackets",
			contactValue: "<sip:bob@example.com:5061>",
			expectedUser: "bob",
			expectedHost: "example.com",
			expectedPort: 5061,
		},
		{
			name:         "Contact without port",
			contactValue: "sip:charlie@test.com",
			expectedUser: "charlie",
			expectedHost: "test.com",
			expectedPort: 0, // sipgo парсер не устанавливает дефолтный порт
		},
		{
			name:         "Contact with parameters",
			contactValue: "<sip:dave@10.0.0.1:5070>",
			expectedUser: "dave",
			expectedHost: "10.0.0.1",
			expectedPort: 5070,
		},
		{
			name:         "Invalid Contact",
			contactValue: "invalid-uri",
			shouldFail:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Создаем минимальный INVITE запрос для тестирования
			req := &sip.Request{
				Method: sip.INVITE,
			}
			req.AppendHeader(sip.NewHeader("Contact", tc.contactValue))

			// Создаем диалог и обрабатываем Contact
			dialog := &Dialog{
				stack: &Stack{
					config: &StackConfig{},
				},
			}

			// Имитируем обработку Contact как в handleIncomingInvite
			if contact := req.GetHeader("Contact"); contact != nil {
				contactStr := contact.Value()
				if strings.HasPrefix(contactStr, "<") && strings.HasSuffix(contactStr, ">") {
					contactStr = strings.TrimPrefix(contactStr, "<")
					contactStr = strings.TrimSuffix(contactStr, ">")
				}

				var contactUri sip.Uri
				err := sip.ParseUri(contactStr, &contactUri)

				if tc.shouldFail {
					if err == nil {
						t.Error("Expected parsing to fail, but it succeeded")
					}
					return
				}

				if err != nil {
					t.Fatalf("Failed to parse Contact URI: %v", err)
				}

				dialog.remoteTarget = contactUri
			}

			// Проверяем результаты
			if dialog.remoteTarget.User != tc.expectedUser {
				t.Errorf("Expected user %s, got %s", tc.expectedUser, dialog.remoteTarget.User)
			}
			if dialog.remoteTarget.Host != tc.expectedHost {
				t.Errorf("Expected host %s, got %s", tc.expectedHost, dialog.remoteTarget.Host)
			}
			if dialog.remoteTarget.Port != tc.expectedPort {
				t.Errorf("Expected port %d, got %d", tc.expectedPort, dialog.remoteTarget.Port)
			}
		})
	}
}

// TestRecordRouteParsing тестирует парсинг Record-Route заголовков
func TestRecordRouteParsing(t *testing.T) {
	tests := []struct {
		name           string
		recordRoutes   []string
		isUAC          bool
		expectedRoutes int
		checkOrder     bool
	}{
		{
			name: "Single Record-Route UAC",
			recordRoutes: []string{
				"<sip:proxy1.example.com;lr>",
			},
			isUAC:          true,
			expectedRoutes: 1,
		},
		{
			name: "Multiple Record-Routes UAC (reverse order)",
			recordRoutes: []string{
				"<sip:proxy1.example.com;lr>",
				"<sip:proxy2.example.com;lr>",
				"<sip:proxy3.example.com;lr>",
			},
			isUAC:          true,
			expectedRoutes: 3,
			checkOrder:     true,
		},
		{
			name: "Multiple Record-Routes UAS (forward order)",
			recordRoutes: []string{
				"<sip:proxy1.example.com;lr>",
				"<sip:proxy2.example.com;lr>",
			},
			isUAC:          false,
			expectedRoutes: 2,
			checkOrder:     true,
		},
		{
			name: "Record-Route with parameters",
			recordRoutes: []string{
				"<sip:proxy.example.com:5060;transport=tcp;lr>",
			},
			isUAC:          true,
			expectedRoutes: 1,
		},
		{
			name: "Invalid Record-Route (skipped)",
			recordRoutes: []string{
				"<sip:proxy1.example.com;lr>",
				"invalid-route",
				"<sip:proxy2.example.com;lr>",
			},
			isUAC:          true,
			expectedRoutes: 2, // только валидные маршруты
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Создаем ответ с Record-Route заголовками
			resp := &sip.Response{
				StatusCode: 200,
			}

			for _, rr := range tc.recordRoutes {
				resp.AppendHeader(sip.NewHeader("Record-Route", rr))
			}

			// Создаем диалог
			dialog := &Dialog{
				isUAC: tc.isUAC,
				stack: &Stack{
					config: &StackConfig{},
				},
			}

			// Обрабатываем Record-Route как в processResponse
			dialog.routeSet = nil
			recordRoutes := resp.GetHeaders("Record-Route")

			if dialog.isUAC {
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
						continue
					}
					dialog.routeSet = append(dialog.routeSet, routeUri)
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
						continue
					}
					dialog.routeSet = append(dialog.routeSet, routeUri)
				}
			}

			// Проверяем количество маршрутов
			if len(dialog.routeSet) != tc.expectedRoutes {
				t.Errorf("Expected %d routes, got %d", tc.expectedRoutes, len(dialog.routeSet))
			}

			// Проверяем порядок для UAC (должен быть обратный)
			if tc.checkOrder && tc.isUAC && len(dialog.routeSet) > 0 {
				// Для UAC первый элемент routeSet должен быть последним Record-Route
				lastIdx := len(tc.recordRoutes) - 1
				for lastIdx >= 0 && !strings.Contains(tc.recordRoutes[lastIdx], "sip:") {
					lastIdx-- // пропускаем невалидные
				}

				if lastIdx >= 0 && strings.Contains(tc.recordRoutes[lastIdx], "proxy3") {
					if !strings.Contains(dialog.routeSet[0].Host, "proxy3") {
						t.Error("UAC should have routes in reverse order")
					}
				}
			}

			// Проверяем порядок для UAS (должен быть прямой)
			if tc.checkOrder && !tc.isUAC && len(dialog.routeSet) > 0 {
				if strings.Contains(tc.recordRoutes[0], "proxy1") {
					if !strings.Contains(dialog.routeSet[0].Host, "proxy1") {
						t.Error("UAS should have routes in forward order")
					}
				}
			}
		})
	}
}

// TestContactParsingInResponse тестирует парсинг Contact в ответах
func TestContactParsingInResponse(t *testing.T) {

	// Создаем стек и диалог
	stack := &Stack{
		config: &StackConfig{},
	}

	dialog := &Dialog{
		stack:    stack,
		isUAC:    true,
		state:    DialogStateRinging,
		localTag: "tag123",
	}

	// Создаем базовый запрос для создания ответа
	req := &sip.Request{
		Method: sip.INVITE,
	}
	req.AppendHeader(sip.NewHeader("From", "Alice <sip:alice@example.com>;tag=1234"))
	req.AppendHeader(sip.NewHeader("To", "Bob <sip:bob@example.com>"))
	req.AppendHeader(sip.NewHeader("Call-ID", "test-call-123"))
	req.AppendHeader(sip.NewHeader("CSeq", "1 INVITE"))

	// Создаем 200 OK ответ с Contact
	resp := sip.NewResponseFromRequest(req, 200, "OK", nil)
	resp.AppendHeader(sip.NewHeader("Contact", "<sip:bob@192.168.1.200:5080>"))
	resp.AppendHeader(sip.NewHeader("Record-Route", "<sip:proxy1.com;lr>"))
	resp.AppendHeader(sip.NewHeader("Record-Route", "<sip:proxy2.com;lr>"))

	// Обрабатываем ответ
	err := dialog.processResponse(resp)
	if err != nil {
		t.Fatalf("Failed to process response: %v", err)
	}

	// Проверяем что Contact был обновлен
	if dialog.remoteTarget.Host != "192.168.1.200" {
		t.Errorf("Expected host 192.168.1.200, got %s", dialog.remoteTarget.Host)
	}
	if dialog.remoteTarget.Port != 5080 {
		t.Errorf("Expected port 5080, got %d", dialog.remoteTarget.Port)
	}

	// Проверяем route set (для UAC должен быть в обратном порядке)
	if len(dialog.routeSet) != 2 {
		t.Errorf("Expected 2 routes, got %d", len(dialog.routeSet))
	}
	if len(dialog.routeSet) > 0 && !strings.Contains(dialog.routeSet[0].Host, "proxy2") {
		t.Error("First route should be proxy2 for UAC (reverse order)")
	}
}
