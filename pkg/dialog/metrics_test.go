package dialog

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"
)

func TestMetricsCollector_Basic(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Logger = GetDefaultLogger().WithComponent("test")
	
	collector := NewMetricsCollector(config)
	if collector == nil {
		t.Fatal("Failed to create metrics collector")
	}
	
	// Тестируем создание диалога
	key := DialogKey{CallID: "test-call", LocalTag: "local", RemoteTag: "remote"}
	collector.DialogCreated(key)
	
	counters := collector.GetPerformanceCounters()
	if counters["total_dialogs"] != 1 {
		t.Errorf("Expected total_dialogs=1, got %d", counters["total_dialogs"])
	}
	if counters["active_dialogs"] != 1 {
		t.Errorf("Expected active_dialogs=1, got %d", counters["active_dialogs"])
	}
	
	// Тестируем завершение диалога
	collector.DialogTerminated(key, "test")
	
	counters = collector.GetPerformanceCounters()
	if counters["total_dialogs"] != 1 {
		t.Errorf("Expected total_dialogs=1, got %d", counters["total_dialogs"])
	}
	if counters["active_dialogs"] != 0 {
		t.Errorf("Expected active_dialogs=0, got %d", counters["active_dialogs"])
	}
}

func TestMetricsCollector_Disabled(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false
	
	collector := NewMetricsCollector(config)
	if collector == nil {
		t.Fatal("Failed to create metrics collector")
	}
	
	// Операции не должны влиять на счетчики
	key := DialogKey{CallID: "test-call", LocalTag: "local", RemoteTag: "remote"}
	collector.DialogCreated(key)
	collector.DialogTerminated(key, "test")
	
	counters := collector.GetPerformanceCounters()
	if counters != nil {
		t.Error("Expected nil counters for disabled collector")
	}
}

func TestMetricsCollector_StateTransitions(t *testing.T) {
	config := DefaultMetricsConfig()
	collector := NewMetricsCollector(config)
	
	// Тестируем переходы состояний
	collector.StateTransition(DialogStateInit, DialogStateTrying, "INVITE_SENT")
	collector.StateTransition(DialogStateTrying, DialogStateRinging, "180_RECEIVED")
	collector.StateTransition(DialogStateRinging, DialogStateEstablished, "200_OK")
	
	// Метрики должны быть зарегистрированы в Prometheus
	// Здесь мы просто проверяем что вызовы не паникуют
}

func TestMetricsCollector_Errors(t *testing.T) {
	config := DefaultMetricsConfig()
	collector := NewMetricsCollector(config)
	
	// Тестируем ошибки
	err := ErrInvalidStateTransition(DialogStateInit, DialogStateTerminated, "Invalid transition")
	collector.ErrorOccurred(err)
	
	counters := collector.GetPerformanceCounters()
	if counters["total_errors"] != 1 {
		t.Errorf("Expected total_errors=1, got %d", counters["total_errors"])
	}
}

func TestMetricsCollector_Recovery(t *testing.T) {
	config := DefaultMetricsConfig()
	collector := NewMetricsCollector(config)
	
	// Тестируем recovery
	collector.Recovery("test_component", "test panic")
	
	counters := collector.GetPerformanceCounters()
	if counters["total_recoveries"] != 1 {
		t.Errorf("Expected total_recoveries=1, got %d", counters["total_recoveries"])
	}
}

func TestMetricsCollector_HealthCheck(t *testing.T) {
	// Создаем простой стек для тестирования
	config := &StackConfig{
		Transport: DefaultTransportConfig(),
		UserAgent: "TestStack/1.0",
	}
	
	stack, err := NewStack(config)
	if err != nil {
		t.Fatalf("Failed to create stack: %v", err)
	}
	
	collector := stack.GetMetrics()
	if collector == nil {
		t.Fatal("Stack should have metrics collector")
	}
	
	// Выполняем health check
	healthCheck := collector.RunHealthCheck(stack)
	if healthCheck == nil {
		t.Fatal("Health check should not be nil")
	}
	
	if healthCheck.Status == HealthUnknown {
		t.Error("Health check status should not be unknown for valid stack")
	}
	
	if len(healthCheck.Components) == 0 {
		t.Error("Health check should report components")
	}
}

func TestMetricsCollector_Integration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Создаем сервер с метриками
	serverConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     8000,
		},
		UserAgent: "MetricsTestServer/1.0",
	}

	serverStack, err := NewStack(serverConfig)
	if err != nil {
		t.Fatalf("Failed to create server stack: %v", err)
	}

	if err := serverStack.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer serverStack.Shutdown(ctx)

	// Создаем клиент
	clientConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     8001,
		},
		UserAgent: "MetricsTestClient/1.0",
	}

	clientStack, err := NewStack(clientConfig)
	if err != nil {
		t.Fatalf("Failed to create client stack: %v", err)
	}

	if err := clientStack.Start(ctx); err != nil {
		t.Fatalf("Failed to start client: %v", err)
	}
	defer clientStack.Shutdown(ctx)

	// Настраиваем колбэк на сервере
	serverStack.OnIncomingDialog(func(d IDialog) {
		if err := d.Accept(ctx); err != nil {
			t.Errorf("Failed to accept: %v", err)
		}
	})

	// Даем время серверу запуститься
	time.Sleep(100 * time.Millisecond)

	// Создаем исходящий вызов
	target := sip.Uri{
		Scheme: "sip",
		User:   "test",
		Host:   "127.0.0.1",
		Port:   8000,
	}

	clientDialog, err := clientStack.NewInvite(ctx, target, InviteOpts{})
	if err != nil {
		t.Fatalf("Failed to create INVITE: %v", err)
	}

	// Ждем ответ
	if d, ok := clientDialog.(*Dialog); ok {
		if err := d.WaitAnswer(ctx); err != nil {
			t.Errorf("Failed to wait answer: %v", err)
		}
	}

	// Проверяем метрики клиента
	clientMetrics := clientStack.GetPerformanceCounters()
	if clientMetrics["total_dialogs"] < 1 {
		t.Errorf("Client: Expected total_dialogs>=1, got %d", clientMetrics["total_dialogs"])
	}

	// Проверяем метрики сервера
	serverMetrics := serverStack.GetPerformanceCounters()
	if serverMetrics["total_dialogs"] < 1 {
		t.Errorf("Server: Expected total_dialogs>=1, got %d", serverMetrics["total_dialogs"])
	}

	// Health check на обоих стеках
	clientHealth := clientStack.RunHealthCheck()
	if clientHealth.Status == HealthUnhealthy {
		t.Errorf("Client health check failed: %v", clientHealth.Errors)
	}

	serverHealth := serverStack.RunHealthCheck()
	if serverHealth.Status == HealthUnhealthy {
		t.Errorf("Server health check failed: %v", serverHealth.Errors)
	}

	// Завершаем вызов
	if err := clientDialog.Bye(ctx, "test"); err != nil {
		t.Errorf("Failed to send BYE: %v", err)
	}

	// Даем время для обработки BYE
	time.Sleep(100 * time.Millisecond)
	
	// Проверяем что активные диалоги стали 0
	finalClientMetrics := clientStack.GetPerformanceCounters()
	if finalClientMetrics["active_dialogs"] != 0 {
		t.Errorf("Client: Expected active_dialogs=0, got %d", finalClientMetrics["active_dialogs"])
	}
}

func TestMetricsCollector_PrometheusIntegration(t *testing.T) {
	// Тест проверяет что Prometheus метрики создаются корректно
	config := DefaultMetricsConfig()
	config.Namespace = "test"
	config.Subsystem = "dialog"
	
	collector := NewMetricsCollector(config)
	
	// Создаем несколько диалогов
	for i := 0; i < 5; i++ {
		key := DialogKey{
			CallID:    fmt.Sprintf("call-%d", i),
			LocalTag:  fmt.Sprintf("local-%d", i),
			RemoteTag: fmt.Sprintf("remote-%d", i),
		}
		collector.DialogCreated(key)
		
		// Имитируем переходы состояний
		collector.StateTransition(DialogStateInit, DialogStateTrying, "INVITE")
		collector.StateTransition(DialogStateTrying, DialogStateEstablished, "200_OK")
		
		// Завершаем половину диалогов
		if i%2 == 0 {
			collector.DialogTerminated(key, "BYE")
		}
	}
	
	// Добавляем транзакции
	for i := 0; i < 10; i++ {
		txID := fmt.Sprintf("tx-%d", i)
		collector.TransactionStarted(txID)
		time.Sleep(time.Millisecond) // Небольшая задержка
		collector.TransactionCompleted(txID, true)
	}
	
	// Добавляем ошибки
	err1 := ErrTransactionTimeout("tx-1", 30*time.Second)
	collector.ErrorOccurred(err1)
	
	err2 := ErrInvalidStateTransition(DialogStateTerminated, DialogStateRinging, "Invalid")
	collector.ErrorOccurred(err2)
	
	// REFER операции
	collector.ReferOperation("send", "success")
	collector.ReferOperation("receive", "rejected")
	
	// Recovery
	collector.Recovery("test_component", "test panic")
	
	// Проверяем счетчики
	counters := collector.GetPerformanceCounters()
	if counters["total_dialogs"] != 5 {
		t.Errorf("Expected total_dialogs=5, got %d", counters["total_dialogs"])
	}
	if counters["active_dialogs"] < 2 { // 5 создано, некоторые могли завершиться
		t.Errorf("Expected active_dialogs>=2, got %d", counters["active_dialogs"])
	}
	if counters["total_transactions"] != 10 {
		t.Errorf("Expected total_transactions=10, got %d", counters["total_transactions"])
	}
	if counters["total_errors"] != 2 {
		t.Errorf("Expected total_errors=2, got %d", counters["total_errors"])
	}
	if counters["total_recoveries"] != 1 {
		t.Errorf("Expected total_recoveries=1, got %d", counters["total_recoveries"])
	}
	
	t.Logf("Final counters: %+v", counters)
}

func TestMetricsCollector_Reset(t *testing.T) {
	config := DefaultMetricsConfig()
	collector := NewMetricsCollector(config)
	
	// Добавляем данные
	key := DialogKey{CallID: "test", LocalTag: "local", RemoteTag: "remote"}
	collector.DialogCreated(key)
	collector.ErrorOccurred(ErrTransactionTimeout("tx-1", 30*time.Second))
	
	// Проверяем что данные есть
	counters := collector.GetPerformanceCounters()
	if counters["total_dialogs"] != 1 {
		t.Errorf("Expected total_dialogs=1, got %d", counters["total_dialogs"])
	}
	
	// Сбрасываем
	collector.Reset()
	
	// Проверяем что данные сброшены
	counters = collector.GetPerformanceCounters()
	if counters["total_dialogs"] != 0 {
		t.Errorf("Expected total_dialogs=0 after reset, got %d", counters["total_dialogs"])
	}
	if counters["total_errors"] != 0 {
		t.Errorf("Expected total_errors=0 after reset, got %d", counters["total_errors"])
	}
}