package dialog

import (
	"fmt"
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"
)

// makeCallIDHeader создает CallIDHeader для тестов
func makeCallIDHeader(callID string) sip.CallIDHeader {
	return sip.CallIDHeader(callID)
}

// BenchmarkGetDialogByTags проверяет производительность поиска по тегам
func BenchmarkGetDialogByTags(b *testing.B) {
	logger := &NoOpLogger{}
	manager := NewDialogManager(logger)
	uasuac := &UASUAC{
		client:     nil,
		server:     nil,
		logger:     logger,
		contactURI: sip.Uri{Scheme: "sip", Host: "localhost", Port: 5060},
	}
	manager.SetUASUAC(uasuac)

	// Создаем тестовые диалоги
	numDialogs := 10000
	dialogs := make([]*Dialog, numDialogs)
	
	for i := 0; i < numDialogs; i++ {
		dialog := &Dialog{
			id:        fmt.Sprintf("dialog-%d", i),
			callID:    makeCallIDHeader(fmt.Sprintf("call-%d", i)),
			localTag:  fmt.Sprintf("local-%d", i),
			remoteTag: fmt.Sprintf("remote-%d", i),
			uasuac:    uasuac,
			logger:    logger,
		}
		dialogs[i] = dialog
		_ = manager.RegisterDialog(dialog)
	}

	// Выбираем случайный диалог для поиска
	targetIdx := numDialogs / 2
	targetDialog := dialogs[targetIdx]

	b.ResetTimer()
	
	// Бенчмарк O(1) поиска по тегам
	b.Run("GetDialogByTags", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := manager.GetDialogByTags(
				(&targetDialog.callID).Value(),
				targetDialog.localTag,
				targetDialog.remoteTag,
			)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	// Для сравнения - старый линейный поиск
	b.Run("LinearSearch", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			found := false
			allDialogs := manager.GetAllDialogs()
			for _, d := range allDialogs {
				dCallID := d.CallID()
				callIDValue := (&dCallID).Value()
				targetCallIDValue := (&targetDialog.callID).Value()
				if callIDValue == targetCallIDValue &&
					d.LocalTag() == targetDialog.localTag &&
					d.RemoteTag() == targetDialog.remoteTag {
					found = true
					break
				}
			}
			if !found {
				b.Fatal("диалог не найден")
			}
		}
	})
}

// BenchmarkDialogOperations проверяет производительность различных операций
func BenchmarkDialogOperations(b *testing.B) {
	logger := &NoOpLogger{}
	manager := NewDialogManager(logger)
	uasuac := &UASUAC{
		client:     nil,
		server:     nil,
		logger:     logger,
		contactURI: sip.Uri{Scheme: "sip", Host: "localhost", Port: 5060},
	}
	manager.SetUASUAC(uasuac)

	b.Run("RegisterDialog", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			dialog := &Dialog{
				id:        fmt.Sprintf("dialog-%d", i),
				callID:    makeCallIDHeader(fmt.Sprintf("call-%d", i)),
				localTag:  fmt.Sprintf("local-%d", i),
				remoteTag: fmt.Sprintf("remote-%d", i),
				uasuac:    uasuac,
				logger:    logger,
			}
			_ = manager.RegisterDialog(dialog)
		}
	})

	// Создаем диалоги для других тестов
	numDialogs := 1000
	for i := 0; i < numDialogs; i++ {
		dialog := &Dialog{
			id:        fmt.Sprintf("test-dialog-%d", i),
			callID:    makeCallIDHeader(fmt.Sprintf("test-call-%d", i)),
			localTag:  fmt.Sprintf("test-local-%d", i),
			remoteTag: fmt.Sprintf("test-remote-%d", i),
			uasuac:    uasuac,
			logger:    logger,
		}
		_ = manager.RegisterDialog(dialog)
	}

	b.Run("GetDialogByCallID", func(b *testing.B) {
		callID := makeCallIDHeader("test-call-500")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = manager.GetDialogByCallID(&callID)
		}
	})

	b.Run("UpdateDialogID", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			oldID := fmt.Sprintf("test-dialog-%d", i%numDialogs)
			newID := fmt.Sprintf("updated-dialog-%d-%d", i%numDialogs, time.Now().UnixNano())
			dialog := &Dialog{
				id:        newID,
				callID:    makeCallIDHeader(fmt.Sprintf("test-call-%d", i%numDialogs)),
				localTag:  fmt.Sprintf("test-local-%d", i%numDialogs),
				remoteTag: fmt.Sprintf("test-remote-%d", i%numDialogs),
				uasuac:    uasuac,
				logger:    logger,
			}
			_ = manager.UpdateDialogID(oldID, newID, dialog)
			// Восстанавливаем для следующей итерации
			_ = manager.UpdateDialogID(newID, oldID, dialog)
		}
	})

	b.Run("RemoveDialog", func(b *testing.B) {
		// Создаем временные диалоги для удаления
		for i := 0; i < b.N; i++ {
			dialog := &Dialog{
				id:        fmt.Sprintf("temp-dialog-%d", i),
				callID:    makeCallIDHeader(fmt.Sprintf("temp-call-%d", i)),
				localTag:  fmt.Sprintf("temp-local-%d", i),
				remoteTag: fmt.Sprintf("temp-remote-%d", i),
				uasuac:    uasuac,
				logger:    logger,
			}
			_ = manager.RegisterDialog(dialog)
		}
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = manager.RemoveDialog(fmt.Sprintf("temp-dialog-%d", i))
		}
	})
}

// TestGetDialogByTags проверяет корректность работы поиска по тегам
func TestGetDialogByTags(t *testing.T) {
	logger := &NoOpLogger{}
	manager := NewDialogManager(logger)
	uasuac := &UASUAC{
		client:     nil,
		server:     nil,
		logger:     logger,
		contactURI: sip.Uri{Scheme: "sip", Host: "localhost", Port: 5060},
	}
	manager.SetUASUAC(uasuac)

	// Создаем тестовый диалог
	dialog := &Dialog{
		id:        "test-dialog-1",
		callID:    makeCallIDHeader("test-call-1"),
		localTag:  "local-tag-1",
		remoteTag: "remote-tag-1",
		uasuac:    uasuac,
		logger:    logger,
	}
	
	err := manager.RegisterDialog(dialog)
	if err != nil {
		t.Fatalf("не удалось зарегистрировать диалог: %v", err)
	}

	// Тест 1: Поиск с правильным порядком тегов
	found, err := manager.GetDialogByTags("test-call-1", "local-tag-1", "remote-tag-1")
	if err != nil {
		t.Errorf("не удалось найти диалог: %v", err)
	}
	if found.ID() != "test-dialog-1" {
		t.Errorf("найден неправильный диалог: ожидался %s, получен %s", "test-dialog-1", found.ID())
	}

	// Тест 2: Поиск с обратным порядком тегов (UAC/UAS перспектива)
	found, err = manager.GetDialogByTags("test-call-1", "remote-tag-1", "local-tag-1")
	if err != nil {
		t.Errorf("не удалось найти диалог с обратным порядком тегов: %v", err)
	}
	if found.ID() != "test-dialog-1" {
		t.Errorf("найден неправильный диалог: ожидался %s, получен %s", "test-dialog-1", found.ID())
	}

	// Тест 3: Поиск несуществующего диалога
	_, err = manager.GetDialogByTags("non-existent", "tag1", "tag2")
	if err == nil {
		t.Error("ожидалась ошибка при поиске несуществующего диалога")
	}

	// Тест 4: Поиск с пустыми параметрами
	_, err = manager.GetDialogByTags("", "tag1", "tag2")
	if err == nil {
		t.Error("ожидалась ошибка при пустом callID")
	}

	_, err = manager.GetDialogByTags("test-call-1", "", "tag2")
	if err == nil {
		t.Error("ожидалась ошибка при пустом localTag")
	}

	_, err = manager.GetDialogByTags("test-call-1", "tag1", "")
	if err == nil {
		t.Error("ожидалась ошибка при пустом remoteTag")
	}
}

// TestTagIndexConsistency проверяет консистентность индекса тегов
func TestTagIndexConsistency(t *testing.T) {
	logger := &NoOpLogger{}
	manager := NewDialogManager(logger)
	uasuac := &UASUAC{
		client:     nil,
		server:     nil,
		logger:     logger,
		contactURI: sip.Uri{Scheme: "sip", Host: "localhost", Port: 5060},
	}
	manager.SetUASUAC(uasuac)

	// Создаем диалог
	dialog := &Dialog{
		id:        "dialog-1",
		callID:    makeCallIDHeader("call-1"),
		localTag:  "local-1",
		remoteTag: "remote-1",
		uasuac:    uasuac,
		logger:    logger,
	}
	
	// Регистрируем диалог
	err := manager.RegisterDialog(dialog)
	if err != nil {
		t.Fatalf("не удалось зарегистрировать диалог: %v", err)
	}

	// Проверяем что диалог найден
	found, err := manager.GetDialogByTags("call-1", "local-1", "remote-1")
	if err != nil || found.ID() != "dialog-1" {
		t.Error("диалог не найден после регистрации")
	}

	// Обновляем ID диалога
	err = manager.UpdateDialogID("dialog-1", "dialog-2", dialog)
	if err != nil {
		t.Fatalf("не удалось обновить ID диалога: %v", err)
	}

	// Проверяем что диалог все еще найден по тегам
	found, err = manager.GetDialogByTags("call-1", "local-1", "remote-1")
	if err != nil || found.ID() != "dialog-2" {
		t.Error("диалог не найден после обновления ID")
	}

	// Удаляем диалог
	err = manager.RemoveDialog("dialog-2")
	if err != nil {
		t.Fatalf("не удалось удалить диалог: %v", err)
	}

	// Проверяем что диалог больше не найден
	_, err = manager.GetDialogByTags("call-1", "local-1", "remote-1")
	if err == nil {
		t.Error("диалог найден после удаления")
	}
}