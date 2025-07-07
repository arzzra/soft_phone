package dialog

import (
	"testing"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessRecordRoute(t *testing.T) {
	tests := []struct {
		name          string
		direction     DialogDirection
		recordRoutes  []string
		expectedURIs  []string
		expectError   bool
	}{
		{
			name:      "UAC with single Record-Route",
			direction: DialogDirectionUAC,
			recordRoutes: []string{
				"<sip:proxy1.example.com;lr>",
			},
			expectedURIs: []string{
				"sip:proxy1.example.com;lr",
			},
		},
		{
			name:      "UAC with multiple Record-Routes",
			direction: DialogDirectionUAC,
			recordRoutes: []string{
				"<sip:proxy1.example.com;lr>, <sip:proxy2.example.com;lr>",
			},
			expectedURIs: []string{
				"sip:proxy1.example.com;lr",
				"sip:proxy2.example.com;lr",
			},
		},
		{
			name:      "UAC with separate Record-Route headers",
			direction: DialogDirectionUAC,
			recordRoutes: []string{
				"<sip:proxy1.example.com;lr>",
				"<sip:proxy2.example.com;lr>",
				"<sip:proxy3.example.com;lr>",
			},
			expectedURIs: []string{
				"sip:proxy1.example.com;lr",
				"sip:proxy2.example.com;lr",
				"sip:proxy3.example.com;lr",
			},
		},
		{
			name:      "UAS with multiple Record-Routes (reversed)",
			direction: DialogDirectionUAS,
			recordRoutes: []string{
				"<sip:proxy1.example.com;lr>, <sip:proxy2.example.com;lr>",
			},
			expectedURIs: []string{
				"sip:proxy2.example.com;lr",
				"sip:proxy1.example.com;lr",
			},
		},
		{
			name:      "UAS with separate headers (reversed)",
			direction: DialogDirectionUAS,
			recordRoutes: []string{
				"<sip:proxy1.example.com;lr>",
				"<sip:proxy2.example.com;lr>",
				"<sip:proxy3.example.com;lr>",
			},
			expectedURIs: []string{
				"sip:proxy3.example.com;lr",
				"sip:proxy2.example.com;lr",
				"sip:proxy1.example.com;lr",
			},
		},
		{
			name:      "Record-Route with display name",
			direction: DialogDirectionUAC,
			recordRoutes: []string{
				"\"Proxy Server\" <sip:proxy.example.com;lr>",
			},
			expectedURIs: []string{
				"sip:proxy.example.com;lr",
			},
		},
		{
			name:      "Record-Route with parameters",
			direction: DialogDirectionUAC,
			recordRoutes: []string{
				"<sip:proxy.example.com;lr;ftag=123;other=value>",
			},
			expectedURIs: []string{
				"sip:proxy.example.com;lr;ftag=123;other=value",
			},
		},
		{
			name:         "Empty Record-Route headers",
			direction:    DialogDirectionUAC,
			recordRoutes: []string{},
			expectedURIs: []string{},
		},
		{
			name:      "Record-Route with IPv6",
			direction: DialogDirectionUAC,
			recordRoutes: []string{
				"<sip:[2001:db8::1];lr>",
			},
			expectedURIs: []string{
				"sip:[2001:db8::1];lr",
			},
		},
		{
			name:      "Record-Route with port",
			direction: DialogDirectionUAC,
			recordRoutes: []string{
				"<sip:proxy.example.com:5061;lr>",
			},
			expectedURIs: []string{
				"sip:proxy.example.com:5061;lr",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Создаем тестовый диалог
			dialog := &sipDialog{
				id: DialogID{
					CallID:    "test-call-id",
					LocalTag:  "local-tag",
					RemoteTag: "remote-tag",
				},
				direction: tt.direction,
				routeSet:  []types.URI{},
			}

			// Создаем тестовый ответ
			resp := types.NewResponse(200, "OK")
			resp.SetHeader(types.HeaderCSeq, "1 INVITE")
			
			// Добавляем Record-Route заголовки
			for _, rr := range tt.recordRoutes {
				resp.AddHeader(types.HeaderRecordRoute, rr)
			}

			// Обрабатываем Record-Route
			err := dialog.ProcessRecordRoute(resp)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Проверяем route set
			assert.Len(t, dialog.routeSet, len(tt.expectedURIs))
			
			for i, expectedURI := range tt.expectedURIs {
				assert.Equal(t, expectedURI, dialog.routeSet[i].String())
			}
		})
	}
}

func TestProcessRecordRoute_OnlyOnce(t *testing.T) {
	// Создаем диалог
	dialog := &sipDialog{
		id: DialogID{
			CallID:    "test-call-id",
			LocalTag:  "local-tag",
			RemoteTag: "remote-tag",
		},
		direction: DialogDirectionUAC,
		routeSet:  []types.URI{},
	}

	// Первый ответ с Record-Route
	resp1 := types.NewResponse(200, "OK")
	resp1.SetHeader(types.HeaderCSeq, "1 INVITE")
	resp1.AddHeader(types.HeaderRecordRoute, "<sip:proxy1.example.com;lr>")

	err := dialog.ProcessRecordRoute(resp1)
	require.NoError(t, err)
	assert.Len(t, dialog.routeSet, 1)
	assert.Equal(t, "sip:proxy1.example.com;lr", dialog.routeSet[0].String())

	// Второй ответ с другим Record-Route (должен быть проигнорирован)
	resp2 := types.NewResponse(200, "OK")
	resp2.SetHeader(types.HeaderCSeq, "2 INVITE")
	resp2.AddHeader(types.HeaderRecordRoute, "<sip:proxy2.example.com;lr>")

	err = dialog.ProcessRecordRoute(resp2)
	require.NoError(t, err)
	
	// Route set не должен измениться
	assert.Len(t, dialog.routeSet, 1)
	assert.Equal(t, "sip:proxy1.example.com;lr", dialog.routeSet[0].String())
}

func TestProcessRecordRoute_ComplexScenario(t *testing.T) {
	// Тест сложного сценария с несколькими прокси
	dialog := &sipDialog{
		id: DialogID{
			CallID:    "complex-call-id",
			LocalTag:  "caller-tag",
			RemoteTag: "callee-tag",
		},
		direction: DialogDirectionUAC,
		routeSet:  []types.URI{},
	}

	// Ответ с несколькими Record-Route заголовками от разных прокси
	resp := types.NewResponse(200, "OK")
	resp.SetHeader(types.HeaderCSeq, "1 INVITE")
	
	// Порядок Record-Route в ответе: от UAC к UAS
	resp.AddHeader(types.HeaderRecordRoute, "<sip:outbound-proxy.caller.com;lr>")
	resp.AddHeader(types.HeaderRecordRoute, "<sip:core-proxy.network.com;lr>, <sip:edge-proxy.network.com;lr>")
	resp.AddHeader(types.HeaderRecordRoute, "<sip:inbound-proxy.callee.com;lr>")

	err := dialog.ProcessRecordRoute(resp)
	require.NoError(t, err)

	// UAC должен использовать маршруты в прямом порядке
	expectedRoutes := []string{
		"sip:outbound-proxy.caller.com;lr",
		"sip:core-proxy.network.com;lr",
		"sip:edge-proxy.network.com;lr",
		"sip:inbound-proxy.callee.com;lr",
	}

	assert.Len(t, dialog.routeSet, len(expectedRoutes))
	for i, expected := range expectedRoutes {
		assert.Equal(t, expected, dialog.routeSet[i].String())
	}
}

func TestUpdateFromResponse_IntegrationWithProcessRecordRoute(t *testing.T) {
	// Тест интеграции updateFromResponse с ProcessRecordRoute
	dialog := &sipDialog{
		id: DialogID{
			CallID:    "integration-call-id",
			LocalTag:  "local-tag",
			RemoteTag: "remote-tag",
		},
		direction: DialogDirectionUAC,
		routeSet:  []types.URI{},
		state:     DialogStateEarly,
	}

	// 200 OK ответ на INVITE с Record-Route
	resp := types.NewResponse(200, "OK")
	resp.SetHeader(types.HeaderCSeq, "1 INVITE")
	resp.AddHeader(types.HeaderRecordRoute, "<sip:proxy1.example.com;lr>, <sip:proxy2.example.com;lr>")
	resp.SetHeader(types.HeaderContact, "<sip:bob@192.168.1.100:5060>")

	// Вызываем updateFromResponse, который должен вызвать ProcessRecordRoute
	err := dialog.updateFromResponse(resp)
	require.NoError(t, err)

	// Проверяем, что route set был установлен
	assert.Len(t, dialog.routeSet, 2)
	assert.Equal(t, "sip:proxy1.example.com;lr", dialog.routeSet[0].String())
	assert.Equal(t, "sip:proxy2.example.com;lr", dialog.routeSet[1].String())
}