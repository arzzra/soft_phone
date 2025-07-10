package adapter

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
	
	"github.com/emiago/sipgo/sip"
	"github.com/arzzra/soft_phone/pkg/dialog"
)

// TestHarness предоставляет инфраструктуру для параллельного тестирования
// старой и новой реализации диалогов
type TestHarness struct {
	// Фабрики для создания диалогов
	legacyFactory  DialogFactory
	enhancedFactory DialogFactory
	
	// Логгер для тестов
	logger dialog.Logger
	
	// Сборщик результатов
	results *TestResults
	mu      sync.Mutex
}

// TestResults результаты тестирования
type TestResults struct {
	// Счетчики
	TotalTests    int
	PassedTests   int
	FailedTests   int
	SkippedTests  int
	
	// Детальные результаты
	TestCases []TestCase
	
	// Метрики производительности
	LegacyMetrics   PerformanceMetrics
	EnhancedMetrics PerformanceMetrics
}

// TestCase представляет один тестовый случай
type TestCase struct {
	Name           string
	Description    string
	Passed         bool
	Skipped        bool
	Error          error
	Duration       time.Duration
	LegacyResult   interface{}
	EnhancedResult interface{}
	Differences    []string
}

// PerformanceMetrics метрики производительности
type PerformanceMetrics struct {
	// Время выполнения операций
	CreateDialogTime    time.Duration
	StateChangeTime     time.Duration
	SendRequestTime     time.Duration
	ProcessRequestTime  time.Duration
	
	// Использование памяти
	MemoryPerDialog     int64
	TotalMemoryUsed     int64
	
	// Счетчики
	DialogsCreated      int
	RequestsSent        int
	RequestsReceived    int
	StateChanges        int
}

// NewTestHarness создает новый тестовый харнесс
func NewTestHarness(logger dialog.Logger) *TestHarness {
	if logger == nil {
		logger = &dialog.NoOpLogger{}
	}
	
	return &TestHarness{
		logger: logger,
		results: &TestResults{
			TestCases: make([]TestCase, 0),
		},
	}
}

// SetLegacyFactory устанавливает фабрику для старой реализации
func (th *TestHarness) SetLegacyFactory(factory DialogFactory) {
	th.legacyFactory = factory
}

// SetEnhancedFactory устанавливает фабрику для новой реализации
func (th *TestHarness) SetEnhancedFactory(factory DialogFactory) {
	th.enhancedFactory = factory
}

// RunTest запускает тест параллельно для обеих реализаций
func (th *TestHarness) RunTest(t *testing.T, name string, testFunc func(d dialog.IDialog) (interface{}, error)) {
	th.mu.Lock()
	defer th.mu.Unlock()
	
	testCase := TestCase{
		Name:        name,
		Description: t.Name(),
	}
	
	th.results.TotalTests++
	
	// Проверяем наличие фабрик
	if th.legacyFactory == nil || th.enhancedFactory == nil {
		testCase.Skipped = true
		testCase.Error = fmt.Errorf("фабрики не установлены")
		th.results.SkippedTests++
		th.results.TestCases = append(th.results.TestCases, testCase)
		t.Skip("Фабрики диалогов не установлены")
		return
	}
	
	// Создаем тестовые диалоги
	callIDValue := fmt.Sprintf("test-%d@harness", time.Now().UnixNano())
	callID := sip.CallIDHeader(callIDValue)
	localTag := fmt.Sprintf("local-%d", time.Now().UnixNano())
	remoteTag := fmt.Sprintf("remote-%d", time.Now().UnixNano())
	
	// Запускаем тесты параллельно
	var wg sync.WaitGroup
	var legacyResult, enhancedResult interface{}
	var legacyErr, enhancedErr error
	var legacyDuration, enhancedDuration time.Duration
	
	// Тест старой реализации
	wg.Add(1)
	go func() {
		defer wg.Done()
		
		startTime := time.Now()
		legacyDialog, err := th.legacyFactory.CreateDialog(callID, localTag, remoteTag, false)
		if err != nil {
			legacyErr = fmt.Errorf("ошибка создания legacy диалога: %w", err)
			return
		}
		defer legacyDialog.Close()
		
		th.results.LegacyMetrics.CreateDialogTime += time.Since(startTime)
		th.results.LegacyMetrics.DialogsCreated++
		
		// Запускаем тест
		testStart := time.Now()
		legacyResult, legacyErr = testFunc(legacyDialog)
		legacyDuration = time.Since(testStart)
	}()
	
	// Тест новой реализации
	wg.Add(1)
	go func() {
		defer wg.Done()
		
		startTime := time.Now()
		enhancedDialog, err := th.enhancedFactory.CreateDialog(callID, localTag, remoteTag, false)
		if err != nil {
			enhancedErr = fmt.Errorf("ошибка создания enhanced диалога: %w", err)
			return
		}
		defer enhancedDialog.Close()
		
		th.results.EnhancedMetrics.CreateDialogTime += time.Since(startTime)
		th.results.EnhancedMetrics.DialogsCreated++
		
		// Запускаем тест
		testStart := time.Now()
		enhancedResult, enhancedErr = testFunc(enhancedDialog)
		enhancedDuration = time.Since(testStart)
	}()
	
	// Ждем завершения
	wg.Wait()
	
	// Сравниваем результаты
	testCase.LegacyResult = legacyResult
	testCase.EnhancedResult = enhancedResult
	testCase.Duration = legacyDuration + enhancedDuration
	
	// Проверяем ошибки
	if legacyErr != nil || enhancedErr != nil {
		if legacyErr != nil && enhancedErr != nil {
			// Обе реализации вернули ошибку - это может быть ок
			if legacyErr.Error() != enhancedErr.Error() {
				testCase.Differences = append(testCase.Differences, 
					fmt.Sprintf("Разные ошибки: legacy=%v, enhanced=%v", legacyErr, enhancedErr))
			}
		} else {
			// Только одна реализация вернула ошибку
			testCase.Error = fmt.Errorf("несоответствие ошибок: legacy=%v, enhanced=%v", legacyErr, enhancedErr)
			testCase.Passed = false
		}
	} else {
		// Сравниваем результаты
		differences := th.compareResults(legacyResult, enhancedResult)
		testCase.Differences = differences
		testCase.Passed = len(differences) == 0
	}
	
	// Обновляем счетчики
	if testCase.Passed {
		th.results.PassedTests++
	} else {
		th.results.FailedTests++
		
		// Логируем различия
		for _, diff := range testCase.Differences {
			t.Errorf("%s: %s", name, diff)
		}
	}
	
	th.results.TestCases = append(th.results.TestCases, testCase)
	
	// Логируем результат
	th.logger.Info("тест завершен",
		dialog.F("test", name),
		dialog.F("passed", testCase.Passed),
		dialog.F("legacy_duration_ms", legacyDuration.Milliseconds()),
		dialog.F("enhanced_duration_ms", enhancedDuration.Milliseconds()),
	)
}

// RunBenchmark запускает бенчмарк для обеих реализаций
func (th *TestHarness) RunBenchmark(b *testing.B, name string, benchFunc func(d dialog.IDialog)) {
	// Бенчмарк старой реализации
	b.Run(name+"/Legacy", func(b *testing.B) {
		if th.legacyFactory == nil {
			b.Skip("Legacy factory not set")
		}
		
		callID := sip.CallIDHeader("bench@legacy")
		d, err := th.legacyFactory.CreateDialog(callID, "local", "remote", false)
		if err != nil {
			b.Fatal(err)
		}
		defer d.Close()
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			benchFunc(d)
		}
	})
	
	// Бенчмарк новой реализации
	b.Run(name+"/Enhanced", func(b *testing.B) {
		if th.enhancedFactory == nil {
			b.Skip("Enhanced factory not set")
		}
		
		callID := sip.CallIDHeader("bench@enhanced")
		d, err := th.enhancedFactory.CreateDialog(callID, "local", "remote", false)
		if err != nil {
			b.Fatal(err)
		}
		defer d.Close()
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			benchFunc(d)
		}
	})
}

// compareResults сравнивает результаты двух реализаций
func (th *TestHarness) compareResults(legacy, enhanced interface{}) []string {
	differences := make([]string, 0)
	
	// Простое сравнение для базовых типов
	if legacy != enhanced {
		differences = append(differences, 
			fmt.Sprintf("Результаты отличаются: legacy=%v, enhanced=%v", legacy, enhanced))
	}
	
	// TODO: Добавить более сложное сравнение для структур
	
	return differences
}

// GetResults возвращает результаты тестирования
func (th *TestHarness) GetResults() *TestResults {
	th.mu.Lock()
	defer th.mu.Unlock()
	
	// Создаем копию результатов
	results := *th.results
	results.TestCases = make([]TestCase, len(th.results.TestCases))
	copy(results.TestCases, th.results.TestCases)
	
	return &results
}

// PrintSummary выводит итоговую информацию о тестировании
func (th *TestHarness) PrintSummary() {
	results := th.GetResults()
	
	fmt.Println("\n=== Итоги тестирования ===")
	fmt.Printf("Всего тестов: %d\n", results.TotalTests)
	fmt.Printf("Успешно: %d (%.1f%%)\n", 
		results.PassedTests, 
		float64(results.PassedTests)/float64(results.TotalTests)*100)
	fmt.Printf("Неудачно: %d\n", results.FailedTests)
	fmt.Printf("Пропущено: %d\n", results.SkippedTests)
	
	// Сравнение производительности
	fmt.Println("\n=== Сравнение производительности ===")
	fmt.Printf("Создание диалога:\n")
	fmt.Printf("  Legacy:   %v\n", results.LegacyMetrics.CreateDialogTime)
	fmt.Printf("  Enhanced: %v\n", results.EnhancedMetrics.CreateDialogTime)
	
	// Детали по неудачным тестам
	if results.FailedTests > 0 {
		fmt.Println("\n=== Неудачные тесты ===")
		for _, tc := range results.TestCases {
			if !tc.Passed && !tc.Skipped {
				fmt.Printf("\n%s:\n", tc.Name)
				if tc.Error != nil {
					fmt.Printf("  Ошибка: %v\n", tc.Error)
				}
				for _, diff := range tc.Differences {
					fmt.Printf("  - %s\n", diff)
				}
			}
		}
	}
}

// CompareDialogBehavior сравнивает поведение двух диалогов
func (th *TestHarness) CompareDialogBehavior(ctx context.Context, scenario string) error {
	th.logger.Info("запуск сценария сравнения", dialog.F("scenario", scenario))
	
	// Создаем диалоги
	callIDValue := fmt.Sprintf("compare-%d", time.Now().UnixNano())
	callID := sip.CallIDHeader(callIDValue)
	
	legacyDialog, err := th.legacyFactory.CreateDialog(callID, "local", "remote", false)
	if err != nil {
		return fmt.Errorf("ошибка создания legacy диалога: %w", err)
	}
	defer legacyDialog.Close()
	
	enhancedDialog, err := th.enhancedFactory.CreateDialog(callID, "local", "remote", false)
	if err != nil {
		return fmt.Errorf("ошибка создания enhanced диалога: %w", err)
	}
	defer enhancedDialog.Close()
	
	// Выполняем сценарий для обоих диалогов
	switch scenario {
	case "basic_lifecycle":
		return th.compareBasicLifecycle(ctx, legacyDialog, enhancedDialog)
		
	case "concurrent_operations":
		return th.compareConcurrentOperations(ctx, legacyDialog, enhancedDialog)
		
	default:
		return fmt.Errorf("неизвестный сценарий: %s", scenario)
	}
}

// compareBasicLifecycle сравнивает базовый жизненный цикл
func (th *TestHarness) compareBasicLifecycle(ctx context.Context, legacy, enhanced dialog.IDialog) error {
	// Проверяем начальное состояние
	if legacy.State() != enhanced.State() {
		return fmt.Errorf("разные начальные состояния: legacy=%v, enhanced=%v", 
			legacy.State(), enhanced.State())
	}
	
	// TODO: Добавить больше проверок жизненного цикла
	
	return nil
}

// compareConcurrentOperations сравнивает поведение при конкурентных операциях
func (th *TestHarness) compareConcurrentOperations(ctx context.Context, legacy, enhanced dialog.IDialog) error {
	// TODO: Реализовать тесты конкурентности
	return nil
}