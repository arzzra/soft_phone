package dialog_test

import (
	"context"
	"fmt"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/dialog"
	"github.com/emiago/sipgo/sip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ExampleBasicTest демонстрирует использование testify для тестирования dialog
func TestExampleBasic(t *testing.T) {
	// Создаем UA1
	cfg1 := dialog.Config{
		Contact:     "contact-UA1",
		DisplayName: "UA1",
		UserAgent:   "TestAgent-UA1",
		TransportConfigs: []dialog.TransportConfig{
			{
				Type: dialog.TransportUDP,
				Host: "127.0.0.1",
				Port: 25060,
			},
		},
		TestMode: true,
	}
	
	ua1, err := dialog.NewUACUAS(cfg1)
	require.NoError(t, err, "Failed to create UA1")
	
	// Создаем UA2
	cfg2 := dialog.Config{
		Contact:     "contact-UA2",
		DisplayName: "UA2",
		UserAgent:   "TestAgent-UA2",
		TransportConfigs: []dialog.TransportConfig{
			{
				Type: dialog.TransportUDP,
				Host: "127.0.0.1",
				Port: 26060,
			},
		},
		TestMode: true,
	}
	
	ua2, err := dialog.NewUACUAS(cfg2)
	require.NoError(t, err, "Failed to create UA2")
	
	// Запускаем транспорты
	ctx := context.Background()
	
	go func() {
		err := ua1.ListenTransports(ctx)
		if err != nil {
			log.Printf("UA1 transport error: %v", err)
		}
	}()
	
	go func() {
		err := ua2.ListenTransports(ctx)
		if err != nil {
			log.Printf("UA2 transport error: %v", err)
		}
	}()
	
	// Даем время на инициализацию
	time.Sleep(500 * time.Millisecond)
	
	// Настраиваем обработчик для UA2
	callReceived := make(chan bool, 1)
	var ua2Dialog dialog.IDialog
	
	ua2.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {
		ua2Dialog = d
		
		// Отправляем 180 Ringing
		err := tx.Provisional(180, "Ringing")
		assert.NoError(t, err, "Failed to send 180")
		
		// Принимаем звонок
		sdp := generateTestSDP(26000)
		err = tx.Accept(dialog.ResponseWithSDP(sdp))
		assert.NoError(t, err, "Failed to accept call")
		
		// Ждем ACK
		go func() {
			err := tx.WaitAck()
			assert.NoError(t, err, "Failed to receive ACK")
			callReceived <- true
		}()
	})
	
	// UA1 инициирует звонок
	d1, err := ua1.NewDialog(ctx)
	require.NoError(t, err, "Failed to create dialog")
	
	sdp := generateTestSDP(25000)
	tx, err := d1.Start(ctx, "sip:user2@127.0.0.1:26060", dialog.WithSDP(sdp))
	require.NoError(t, err, "Failed to start call")
	
	// Ждем ответы
	responseCount := 0
	timeout := time.After(5 * time.Second)
	
	for responseCount < 2 {
		select {
		case resp := <-tx.Responses():
			require.NotNil(t, resp, "Received nil response")
			responseCount++
			
			if responseCount == 1 {
				assert.Equal(t, 180, resp.StatusCode, "Expected 180 Ringing")
			} else if responseCount == 2 {
				assert.Equal(t, 200, resp.StatusCode, "Expected 200 OK")
			}
		case <-timeout:
			t.Fatal("Timeout waiting for responses")
		}
	}
	
	// Ждем установления соединения
	select {
	case <-callReceived:
		log.Println("Call established successfully")
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for call establishment")
	}
	
	// Проверяем состояние диалогов
	assert.Equal(t, dialog.InCall, d1.State(), "UA1 should be InCall")
	assert.Equal(t, dialog.InCall, ua2Dialog.State(), "UA2 should be InCall")
	
	// Завершаем звонок
	err = d1.Terminate()
	assert.NoError(t, err, "Failed to terminate call")
	
	// Даем время на завершение
	time.Sleep(500 * time.Millisecond)
}

// TestConcurrentCallsWithTestify демонстрирует параллельные звонки
func TestConcurrentCallsWithTestify(t *testing.T) {
	numCalls := 3
	basePort1 := 30000
	basePort2 := 35000
	
	type callPair struct {
		ua1    *dialog.UACUAS
		ua2    *dialog.UACUAS
		dialog dialog.IDialog
	}
	
	calls := make([]callPair, numCalls)
	ctx := context.Background()
	
	// Создаем пары UA
	for i := 0; i < numCalls; i++ {
		// UA1
		cfg1 := dialog.Config{
			Contact:     fmt.Sprintf("contact-UA1-%d", i),
			DisplayName: fmt.Sprintf("UA1-%d", i),
			UserAgent:   fmt.Sprintf("TestAgent-UA1-%d", i),
			TransportConfigs: []dialog.TransportConfig{
				{
					Type: dialog.TransportUDP,
					Host: "127.0.0.1",
					Port: basePort1 + i,
				},
			},
			TestMode: true,
		}
		
		ua1, err := dialog.NewUACUAS(cfg1)
		require.NoError(t, err, "Failed to create UA1-%d", i)
		calls[i].ua1 = ua1
		
		// UA2
		cfg2 := dialog.Config{
			Contact:     fmt.Sprintf("contact-UA2-%d", i),
			DisplayName: fmt.Sprintf("UA2-%d", i),
			UserAgent:   fmt.Sprintf("TestAgent-UA2-%d", i),
			TransportConfigs: []dialog.TransportConfig{
				{
					Type: dialog.TransportUDP,
					Host: "127.0.0.1",
					Port: basePort2 + i,
				},
			},
			TestMode: true,
		}
		
		ua2, err := dialog.NewUACUAS(cfg2)
		require.NoError(t, err, "Failed to create UA2-%d", i)
		calls[i].ua2 = ua2
		
		// Запускаем транспорты
		go ua1.ListenTransports(ctx)
		go ua2.ListenTransports(ctx)
	}
	
	// Даем время на инициализацию
	time.Sleep(500 * time.Millisecond)
	
	// Запускаем звонки параллельно
	var wg sync.WaitGroup
	
	for i := 0; i < numCalls; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			
			call := &calls[index]
			
			// Настраиваем обработчик для UA2
			callEstablished := make(chan bool, 1)
			
			call.ua2.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {
				// Принимаем звонок
				sdp := generateTestSDP(basePort2 + index + 1000)
				err := tx.Accept(dialog.ResponseWithSDP(sdp))
				assert.NoError(t, err, "Call %d: Failed to accept", index)
				
				go func() {
					tx.WaitAck()
					callEstablished <- true
				}()
			})
			
			// Инициируем звонок
			d, err := call.ua1.NewDialog(ctx)
			require.NoError(t, err, "Call %d: Failed to create dialog", index)
			call.dialog = d
			
			sdp := generateTestSDP(basePort1 + index + 1000)
			target := fmt.Sprintf("sip:user2@127.0.0.1:%d", basePort2+index)
			_, err = d.Start(ctx, target, dialog.WithSDP(sdp))
			require.NoError(t, err, "Call %d: Failed to start", index)
			
			// Ждем установления
			select {
			case <-callEstablished:
				t.Logf("Call %d established", index)
			case <-time.After(5 * time.Second):
				t.Errorf("Call %d: Timeout", index)
			}
		}(i)
	}
	
	// Ждем завершения всех звонков
	wg.Wait()
	
	// Проверяем, что все звонки установлены
	for i, call := range calls {
		if call.dialog != nil {
			assert.Equal(t, dialog.InCall, call.dialog.State(), 
				"Call %d should be InCall", i)
		}
	}
	
	// Завершаем все звонки
	for i, call := range calls {
		if call.dialog != nil {
			err := call.dialog.Terminate()
			assert.NoError(t, err, "Failed to terminate call %d", i)
		}
	}
	
	time.Sleep(500 * time.Millisecond)
}

// TestWithMockAssertions демонстрирует использование кастомных assertions
func TestWithMockAssertions(t *testing.T) {
	// Пример базовых assertions
	statusCode := 200
	reason := "OK"
	
	assert.Equal(t, 200, statusCode, "Status code should be 200")
	assert.Equal(t, "OK", reason, "Reason should be OK")
	
	// Пример require (останавливает тест при ошибке)
	require.NotNil(t, reason, "Reason should not be nil")
	
	// Пример с условиями
	if assert.True(t, statusCode >= 200 && statusCode < 300) {
		t.Log("Response is successful")
	}
	
	// Пример проверки в цикле
	statusCodes := []int{100, 180, 200}
	for _, code := range statusCodes {
		assert.Contains(t, []int{100, 180, 183, 200}, code, 
			"Status code %d should be valid", code)
	}
}

// Вспомогательная функция для генерации SDP
func generateTestSDP(port int) string {
	return fmt.Sprintf(`v=0
o=- %d %d IN IP4 127.0.0.1
s=Test Session
c=IN IP4 127.0.0.1
t=0 0
m=audio %d RTP/AVP 0 8 101
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000
a=rtpmap:101 telephone-event/8000
a=fmtp:101 0-15
a=sendrecv
`, time.Now().Unix(), time.Now().Unix(), port)
}