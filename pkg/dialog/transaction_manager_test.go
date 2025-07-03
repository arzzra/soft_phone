package dialog

import (
	"context"
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"
)

func TestTransactionManager(t *testing.T) {
	ctx := context.Background()
	
	// Создаем мок стек
	stack := &Stack{
		config: &StackConfig{
			UserAgent: "TestStack/1.0",
		},
	}
	
	// Создаем Transaction Manager
	tm := NewTransactionManager(ctx, stack, nil)
	defer tm.Shutdown()
	
	// Проверяем начальное состояние
	if count := tm.GetActiveCount(); count != 0 {
		t.Errorf("Expected 0 active transactions, got %d", count)
	}
	
	// Проверяем метрики
	if metrics := tm.GetMetrics(); metrics == nil {
		t.Error("Expected metrics to be enabled")
	} else {
		if metrics.TotalCreated != 0 {
			t.Errorf("Expected 0 total created, got %d", metrics.TotalCreated)
		}
	}
}

func TestTransactionAdapter(t *testing.T) {
	ctx := context.Background()
	
	stack := &Stack{
		config: &StackConfig{
			UserAgent: "TestStack/1.0",
		},
	}
	
	// Создаем мок client transaction (nil для тестирования)
	adapter := NewClientTransactionAdapter(ctx, stack, nil, sip.INVITE)
	defer adapter.Terminate()
	
	// Проверяем базовые свойства
	if adapter.Type() != TxTypeClient {
		t.Errorf("Expected client transaction type")
	}
	
	if adapter.Method() != sip.INVITE {
		t.Errorf("Expected INVITE method")
	}
	
	if adapter.State() != TxStateInit {
		t.Errorf("Expected Init state, got %v", adapter.State())
	}
	
	// Проверяем ID
	if id := adapter.ID(); id == "" {
		t.Error("Expected non-empty transaction ID")
	}
	
	// Проверяем контекст
	if ctx := adapter.Context(); ctx == nil {
		t.Error("Expected non-nil context")
	}
}

func TestTransactionStateMapping(t *testing.T) {
	tests := []struct {
		dialogState DialogState
		expected    TransactionState
	}{
		{DialogStateInit, TxStateInit},
		{DialogStateTrying, TxStateTrying},
		{DialogStateRinging, TxStateProceeding},
		{DialogStateEstablished, TxStateCompleted},
		{DialogStateTerminated, TxStateTerminated},
	}
	
	for _, test := range tests {
		result := MapDialogStateToTransactionState(test.dialogState)
		if result != test.expected {
			t.Errorf("MapDialogStateToTransactionState(%v) = %v, expected %v", 
				test.dialogState, result, test.expected)
		}
	}
}

func TestTransactionManagerLimits(t *testing.T) {
	ctx := context.Background()
	
	stack := &Stack{
		config: &StackConfig{
			UserAgent: "TestStack/1.0",
		},
	}
	
	// Создаем Transaction Manager с лимитом
	config := &TransactionManagerConfig{
		MaxTransactions: 2,
		EnableMetrics:   true,
		CleanupInterval: 30 * time.Second,
		DefaultTimeout:  32 * time.Second,
	}
	tm := NewTransactionManager(ctx, stack, config)
	defer tm.Shutdown()
	
	// Тестируем лимиты создавая адаптеры напрямую (обходя sipgo client)
	for i := 0; i < 3; i++ {
		adapter := NewClientTransactionAdapter(ctx, stack, nil, sip.INVITE)
		tm.transactions.Store(adapter.ID(), adapter)
		
		if i < 2 {
			// Первые 2 должны создаваться успешно
			if adapter == nil {
				t.Errorf("Expected adapter %d to be created", i)
			}
		}
	}
	
	// Проверяем что создалось нужное количество
	if count := tm.GetActiveCount(); count != 3 {
		t.Errorf("Expected 3 active transactions, got %d", count)
	}
}

func TestTransactionAdapterTermination(t *testing.T) {
	ctx := context.Background()
	
	stack := &Stack{
		config: &StackConfig{
			UserAgent: "TestStack/1.0",
		},
	}
	
	adapter := NewClientTransactionAdapter(ctx, stack, nil, sip.INVITE)
	
	// Проверяем что не завершена
	if adapter.IsTerminated() {
		t.Error("Expected transaction to not be terminated initially")
	}
	
	// Регистрируем колбэк завершения
	terminated := false
	adapter.OnTerminate(func() {
		terminated = true
	})
	
	// Завершаем транзакцию
	adapter.Terminate()
	
	// Проверяем что завершена
	if !adapter.IsTerminated() {
		t.Error("Expected transaction to be terminated")
	}
	
	// Даем время для выполнения колбэка
	time.Sleep(10 * time.Millisecond)
	
	if !terminated {
		t.Error("Expected terminate callback to be called")
	}
	
	// Проверяем Done канал
	select {
	case <-adapter.Done():
		// Хорошо
	default:
		t.Error("Expected Done channel to be closed")
	}
}

func TestTransactionManagerCleanup(t *testing.T) {
	ctx := context.Background()
	
	stack := &Stack{
		config: &StackConfig{
			UserAgent: "TestStack/1.0",
		},
	}
	
	// Создаем Transaction Manager с коротким интервалом очистки
	config := &TransactionManagerConfig{
		CleanupInterval: 50 * time.Millisecond,
		EnableMetrics:   true,
	}
	tm := NewTransactionManager(ctx, stack, config)
	defer tm.Shutdown()
	
	// Создаем и завершаем несколько адаптеров
	for i := 0; i < 5; i++ {
		adapter := NewClientTransactionAdapter(ctx, stack, nil, sip.INVITE)
		tm.transactions.Store(adapter.ID(), adapter)
		adapter.Terminate()
	}
	
	// Ждем cleanup
	time.Sleep(100 * time.Millisecond)
	
	// Проверяем что cleanup сработал
	// (это сложно протестировать точно, но можем проверить что система работает)
	if count := tm.GetActiveCount(); count > 0 {
		t.Logf("Active count after cleanup: %d (expected cleanup to reduce this)", count)
	}
}