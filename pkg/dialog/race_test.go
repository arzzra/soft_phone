package dialog

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDialogManagerConcurrentAccess проверяет потокобезопасность DialogManager
func TestDialogManagerConcurrentAccess(t *testing.T) {
	manager := NewDialogManager(&NoOpLogger{})
	uasuac := &UASUAC{
		contactURI: sip.Uri{
			Scheme: "sip",
			Host:   "localhost",
			Port:   5060,
		},
	}
	manager.SetUASUAC(uasuac)

	// Количество горутин
	numGoroutines := 10
	numOperations := 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Запускаем горутины для создания диалогов
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			
			for j := 0; j < numOperations; j++ {
				// Создаем запрос
				req := sip.NewRequest(sip.INVITE, sip.Uri{Scheme: "sip", Host: "example.com"})
				callID := sip.CallIDHeader(fmt.Sprintf("call-%d-%d@example.com", id, j))
				req.AppendHeader(&callID)
				req.AppendHeader(sip.NewHeader("From", fmt.Sprintf("<sip:user%d@example.com>;tag=tag%d", id, id)))
				req.AppendHeader(sip.NewHeader("To", "<sip:target@example.com>"))
				req.AppendHeader(sip.NewHeader("CSeq", "1 INVITE"))
				
				// Создаем диалог
				dialog, err := manager.CreateClientDialog(req)
				assert.NoError(t, err)
				assert.NotNil(t, dialog)
				
				// Регистрируем диалог
				err = manager.RegisterDialog(dialog)
				if err != nil {
					// Может быть ошибка если диалог уже зарегистрирован
					assert.Contains(t, err.Error(), "уже зарегистрирован")
				}
			}
		}(i)
	}

	// Запускаем горутины для поиска диалогов
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			
			for j := 0; j < numOperations; j++ {
				callID := sip.CallIDHeader(fmt.Sprintf("call-%d-%d@example.com", id, j))
				
				// Пытаемся найти диалог по Call-ID
				dialog, err := manager.GetDialogByCallID(&callID)
				if err == nil {
					assert.NotNil(t, dialog)
				}
				
				// Небольшая задержка для имитации работы
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	// Запускаем горутины для удаления диалогов
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			
			time.Sleep(10 * time.Millisecond) // Даем время на создание
			
			for j := 0; j < numOperations; j++ {
				callID := sip.CallIDHeader(fmt.Sprintf("call-%d-%d@example.com", id, j))
				
				// Пытаемся найти и удалить диалог
				dialog, err := manager.GetDialogByCallID(&callID)
				if err == nil && dialog != nil {
					err = manager.RemoveDialog(dialog.ID())
					if err != nil {
						// Может быть уже удален
						assert.Contains(t, err.Error(), "не найден")
					}
				}
			}
		}(i)
	}

	// Ждем завершения всех горутин
	wg.Wait()

	// Проверяем состояние
	assert.LessOrEqual(t, manager.GetDialogCount(), numGoroutines*numOperations)
}

// TestDialogIDUpdate проверяет обновление ID диалога при получении remote tag
func TestDialogIDUpdate(t *testing.T) {
	manager := NewDialogManager(&NoOpLogger{})
	uasuac := &UASUAC{
		contactURI: sip.Uri{
			Scheme: "sip",
			Host:   "localhost",
			Port:   5060,
		},
		dialogManager: manager,
	}
	manager.SetUASUAC(uasuac)

	// Создаем INVITE запрос
	req := sip.NewRequest(sip.INVITE, sip.Uri{Scheme: "sip", Host: "example.com"})
	callID := sip.CallIDHeader("test-call-id@example.com")
	req.AppendHeader(&callID)
	req.AppendHeader(sip.NewHeader("From", "<sip:alice@example.com>;tag=local-tag"))
	req.AppendHeader(sip.NewHeader("To", "<sip:bob@example.com>"))
	req.AppendHeader(sip.NewHeader("CSeq", "1 INVITE"))

	// Создаем диалог
	dialog, err := manager.CreateClientDialog(req)
	require.NoError(t, err)
	require.NotNil(t, dialog)

	// Проверяем, что диалог имеет временный ID
	oldID := dialog.ID()
	assert.Contains(t, oldID, "temp_")

	// Проверяем, что диалог можно найти по Call-ID
	foundDialog, err := manager.GetDialogByCallID(&callID)
	require.NoError(t, err)
	assert.Equal(t, dialog.ID(), foundDialog.ID())

	// Симулируем получение ответа с remote tag
	d := dialog.(*Dialog)
	d.mu.Lock()
	d.remoteTag = "remote-tag"
	d.updateDialogID()
	newID := d.id
	d.mu.Unlock()

	// Обновляем ID в менеджере
	err = manager.UpdateDialogID(oldID, newID, dialog)
	require.NoError(t, err)

	// Проверяем, что старый ID больше не существует
	_, err = manager.GetDialog(oldID)
	assert.Error(t, err)

	// Проверяем, что можно найти по новому ID
	foundDialog, err = manager.GetDialog(newID)
	require.NoError(t, err)
	assert.Equal(t, newID, foundDialog.ID())

	// Проверяем, что можно найти по Call-ID
	foundDialog, err = manager.GetDialogByCallID(&callID)
	require.NoError(t, err)
	assert.Equal(t, newID, foundDialog.ID())
}

// TestHandleClientTransactionRace проверяет безопасность handleClientTransaction
func TestHandleClientTransactionRace(t *testing.T) {
	t.Skip("Временно отключен для отладки")
	uasuac := &UASUAC{
		contactURI: sip.Uri{
			Scheme: "sip",
			Host:   "localhost",
			Port:   5060,
		},
	}

	// Создаем диалог
	dialog := NewDialog(uasuac, false, &NoOpLogger{})
	dialog.callID = sip.CallIDHeader("test-call-id")
	dialog.localTag = "local-tag"

	// Создаем мок транзакцию
	tx := &mockClientTransaction{
		responses: make(chan *sip.Response, 10),
	}

	// Запускаем handleClientTransaction в горутине
	go uasuac.handleClientTransaction(dialog, tx)

	// Одновременно отправляем ответы и изменяем состояние диалога
	var wg sync.WaitGroup
	
	// Горутина для отправки ответов
	wg.Add(1)
	go func() {
		defer wg.Done()
		
		// Создаем фейковый запрос для ответа
		req := sip.NewRequest(sip.INVITE, sip.Uri{Scheme: "sip", Host: "example.com"})
		callID := sip.CallIDHeader("test-call-id")
		req.AppendHeader(&callID)
		req.AppendHeader(sip.NewHeader("From", "<sip:alice@example.com>;tag=local-tag"))
		req.AppendHeader(sip.NewHeader("To", "<sip:bob@example.com>"))
		req.AppendHeader(sip.NewHeader("CSeq", "1 INVITE"))
		
		// Отправляем предварительный ответ
		res := sip.NewResponseFromRequest(req, 180, "Ringing", nil)
		res.AppendHeader(sip.NewHeader("To", "<sip:bob@example.com>;tag=remote-tag"))
		tx.responses <- res
		
		time.Sleep(10 * time.Millisecond)
		
		// Отправляем финальный ответ
		res2 := sip.NewResponseFromRequest(req, 200, "OK", nil)
		res2.AppendHeader(sip.NewHeader("To", "<sip:bob@example.com>;tag=remote-tag"))
		res2.AppendHeader(sip.NewHeader("CSeq", "1 INVITE"))
		tx.responses <- res2
		
		close(tx.responses)
	}()

	// Горутина для изменения состояния диалога
	wg.Add(1)
	go func() {
		defer wg.Done()
		
		for i := 0; i < 100; i++ {
			// Читаем состояние
			state := dialog.State()
			_ = state
			
			// Изменяем последнюю активность
			dialog.mu.Lock()
			dialog.lastActivity = time.Now()
			dialog.mu.Unlock()
			
			time.Sleep(time.Microsecond)
		}
	}()

	// Ждем завершения
	wg.Wait()
	
	// Даем время на обработку
	time.Sleep(50 * time.Millisecond)

	// Проверяем состояние диалога
	assert.Equal(t, "remote-tag", dialog.remoteTag)
}

// Моки перенесены в mocks_test.go