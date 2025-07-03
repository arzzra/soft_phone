package dialog

import (
	"context"
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

// RecoveryHandler обработчик для восстановления после паник
type RecoveryHandler interface {
	// HandlePanic обрабатывает панику
	HandlePanic(ctx context.Context, panicValue interface{}, stack []byte, component string)
	
	// ShouldRestart определяет, нужно ли перезапускать компонент
	ShouldRestart(component string, panicCount int) bool
	
	// OnComponentRestart вызывается при перезапуске компонента
	OnComponentRestart(component string, attempt int)
}

// DefaultRecoveryHandler стандартная реализация recovery handler
type DefaultRecoveryHandler struct {
	logger        StructuredLogger
	maxRetries    int
	retryInterval time.Duration
	
	// Статистика паник
	panicCounts   sync.Map // map[string]*int64 - счетчики паник по компонентам
	lastPanic     sync.Map // map[string]time.Time - время последней паники
	
	// Конфигурация
	enableAutoRestart bool
	panicThreshold    int           // Максимум паник за период
	panicWindow       time.Duration // Временное окно для подсчета паник
}

// NewDefaultRecoveryHandler создает новый recovery handler
func NewDefaultRecoveryHandler(logger StructuredLogger) *DefaultRecoveryHandler {
	return &DefaultRecoveryHandler{
		logger:            logger,
		maxRetries:        3,
		retryInterval:     5 * time.Second,
		enableAutoRestart: true,
		panicThreshold:    5,
		panicWindow:       10 * time.Minute,
	}
}

// HandlePanic обрабатывает панику
func (h *DefaultRecoveryHandler) HandlePanic(ctx context.Context, panicValue interface{}, stack []byte, component string) {
	// Увеличиваем счетчик паник
	panicCount := h.incrementPanicCount(component)
	
	// Логируем панику с полной информацией
	h.logger.Error(ctx, fmt.Sprintf("PANIC восстановлен в компоненте %s", component),
		String("component", component),
		Any("panic_value", panicValue),
		String("stack_trace", string(stack)),
		Int64("panic_count", panicCount),
		Time("panic_time", time.Now()),
	)
	
	// Проверяем, не превышен ли лимит паник
	if h.isPanicThresholdExceeded(component) {
		h.logger.Error(ctx, fmt.Sprintf("Превышен лимит паник для компонента %s, отключаем автоперезапуск", component),
			String("component", component),
			Int("threshold", h.panicThreshold),
			Duration("window", h.panicWindow),
		)
		h.enableAutoRestart = false
	}
}

// ShouldRestart определяет, нужно ли перезапускать компонент
func (h *DefaultRecoveryHandler) ShouldRestart(component string, panicCount int) bool {
	if !h.enableAutoRestart {
		return false
	}
	
	if panicCount > h.maxRetries {
		h.logger.Warn(context.Background(), fmt.Sprintf("Достигнут лимит перезапусков для %s", component),
			String("component", component),
			Int("panic_count", panicCount),
			Int("max_retries", h.maxRetries),
		)
		return false
	}
	
	return true
}

// OnComponentRestart вызывается при перезапуске компонента
func (h *DefaultRecoveryHandler) OnComponentRestart(component string, attempt int) {
	h.logger.Info(context.Background(), fmt.Sprintf("Перезапуск компонента %s (попытка %d)", component, attempt),
		String("component", component),
		Int("attempt", attempt),
		Duration("retry_interval", h.retryInterval),
	)
}

// incrementPanicCount увеличивает счетчик паник для компонента
func (h *DefaultRecoveryHandler) incrementPanicCount(component string) int64 {
	value, _ := h.panicCounts.LoadOrStore(component, new(int64))
	counter := value.(*int64)
	count := atomic.AddInt64(counter, 1)
	
	// Обновляем время последней паники
	h.lastPanic.Store(component, time.Now())
	
	return count
}

// isPanicThresholdExceeded проверяет, превышен ли лимит паник
func (h *DefaultRecoveryHandler) isPanicThresholdExceeded(component string) bool {
	value, exists := h.panicCounts.Load(component)
	if !exists {
		return false
	}
	
	counter := value.(*int64)
	count := atomic.LoadInt64(counter)
	
	lastPanicTime, exists := h.lastPanic.Load(component)
	if !exists {
		return false
	}
	
	// Проверяем, сколько паник было за последний период
	since := time.Since(lastPanicTime.(time.Time))
	if since > h.panicWindow {
		// Сбрасываем счетчик если прошло много времени
		atomic.StoreInt64(counter, 0)
		return false
	}
	
	return count >= int64(h.panicThreshold)
}

// SafeGoroutine структура для безопасного запуска горутин с recovery
type SafeGoroutine struct {
	name           string
	recoveryHandler RecoveryHandler
	logger         StructuredLogger
	restartable    bool
	maxRestarts    int
	restartCount   int64
	
	// Контекст и контроль
	ctx            context.Context
	cancel         context.CancelFunc
	done           chan struct{}
	
	// Функция для выполнения
	workFunc       func(ctx context.Context) error
}

// SafeGoroutineOptions опции для SafeGoroutine
type SafeGoroutineOptions struct {
	Name            string
	RecoveryHandler RecoveryHandler
	Logger          StructuredLogger
	Restartable     bool
	MaxRestarts     int
}

// NewSafeGoroutine создает новую безопасную горутину
func NewSafeGoroutine(ctx context.Context, workFunc func(ctx context.Context) error, opts SafeGoroutineOptions) *SafeGoroutine {
	goroutineCtx, cancel := context.WithCancel(ctx)
	
	if opts.Logger == nil {
		opts.Logger = GetDefaultLogger()
	}
	
	if opts.RecoveryHandler == nil {
		opts.RecoveryHandler = NewDefaultRecoveryHandler(opts.Logger)
	}
	
	if opts.MaxRestarts == 0 {
		opts.MaxRestarts = 3
	}
	
	return &SafeGoroutine{
		name:            opts.Name,
		recoveryHandler: opts.RecoveryHandler,
		logger:          opts.Logger,
		restartable:     opts.Restartable,
		maxRestarts:     opts.MaxRestarts,
		ctx:             goroutineCtx,
		cancel:          cancel,
		done:            make(chan struct{}),
		workFunc:        workFunc,
	}
}

// Start запускает безопасную горутину
func (sg *SafeGoroutine) Start() {
	go sg.run()
}

// Stop останавливает горутину
func (sg *SafeGoroutine) Stop() {
	sg.cancel()
	<-sg.done
}

// Wait ожидает завершения горутины
func (sg *SafeGoroutine) Wait() {
	<-sg.done
}

// GetRestartCount возвращает количество перезапусков
func (sg *SafeGoroutine) GetRestartCount() int64 {
	return atomic.LoadInt64(&sg.restartCount)
}

// run основной цикл выполнения
func (sg *SafeGoroutine) run() {
	defer close(sg.done)
	
	sg.logger.Debug(sg.ctx, fmt.Sprintf("Запуск безопасной горутины: %s", sg.name),
		String("goroutine", sg.name),
		Bool("restartable", sg.restartable),
		Int("max_restarts", sg.maxRestarts),
	)
	
	for {
		select {
		case <-sg.ctx.Done():
			sg.logger.Debug(sg.ctx, fmt.Sprintf("Горутина %s остановлена по контексту", sg.name),
				String("goroutine", sg.name),
			)
			return
		default:
			sg.runWithRecovery()
			
			// Если не перезапускаемая или достигнут лимит - выходим
			if !sg.restartable || !sg.shouldRestart() {
				return
			}
			
			// Задержка перед перезапуском
			select {
			case <-sg.ctx.Done():
				return
			case <-time.After(5 * time.Second):
				// Продолжаем
			}
		}
	}
}

// runWithRecovery выполняет работу с защитой от паник
func (sg *SafeGoroutine) runWithRecovery() {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			
			// Обрабатываем панику через handler
			sg.recoveryHandler.HandlePanic(sg.ctx, r, stack, sg.name)
			
			// Увеличиваем счетчик перезапусков
			atomic.AddInt64(&sg.restartCount, 1)
		}
	}()
	
	// Выполняем основную работу
	if err := sg.workFunc(sg.ctx); err != nil {
		sg.logger.LogError(sg.ctx, err, fmt.Sprintf("Ошибка в горутине %s", sg.name),
			String("goroutine", sg.name),
		)
	}
}

// shouldRestart определяет, нужно ли перезапускать горутину
func (sg *SafeGoroutine) shouldRestart() bool {
	restartCount := atomic.LoadInt64(&sg.restartCount)
	
	if int(restartCount) >= sg.maxRestarts {
		sg.logger.Error(sg.ctx, fmt.Sprintf("Достигнут лимит перезапусков для горутины %s", sg.name),
			String("goroutine", sg.name),
			Int64("restart_count", restartCount),
			Int("max_restarts", sg.maxRestarts),
		)
		return false
	}
	
	shouldRestart := sg.recoveryHandler.ShouldRestart(sg.name, int(restartCount))
	if shouldRestart {
		sg.recoveryHandler.OnComponentRestart(sg.name, int(restartCount))
	}
	
	return shouldRestart
}

// SafeExecute выполняет функцию с защитой от паник
func SafeExecute(ctx context.Context, name string, fn func() error, logger StructuredLogger) error {
	var err error
	
	func() {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				
				if logger != nil {
					logger.Error(ctx, fmt.Sprintf("PANIC в %s восстановлен", name),
						String("component", name),
						Any("panic_value", r),
						String("stack_trace", string(stack)),
					)
				}
				
				// Преобразуем панику в ошибку
				err = NewDialogError(
					"PANIC_RECOVERED",
					fmt.Sprintf("Восстановлена паника в %s: %v", name, r),
					ErrorCategorySystem,
					ErrorSeverityCritical,
				).WithField("panic_value", r).WithField("stack_trace", string(stack))
			}
		}()
		
		err = fn()
	}()
	
	return err
}

// SafeExecuteWithRetry выполняет функцию с повторными попытками
func SafeExecuteWithRetry(ctx context.Context, name string, fn func() error, maxRetries int, interval time.Duration, logger StructuredLogger) error {
	var lastErr error
	
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Задержка между попытками
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(interval):
			}
			
			if logger != nil {
				logger.Debug(ctx, fmt.Sprintf("Повторная попытка %s (попытка %d/%d)", name, attempt+1, maxRetries+1),
					String("component", name),
					Int("attempt", attempt+1),
					Int("max_attempts", maxRetries+1),
				)
			}
		}
		
		err := SafeExecute(ctx, name, fn, logger)
		if err == nil {
			// Успех
			if attempt > 0 && logger != nil {
				logger.Info(ctx, fmt.Sprintf("Операция %s успешна после %d попыток", name, attempt+1),
					String("component", name),
					Int("attempts", attempt+1),
				)
			}
			return nil
		}
		
		lastErr = err
		
		// Проверяем, стоит ли повторять
		if !IsTemporary(err) && attempt == 0 {
			if logger != nil {
				logger.Debug(ctx, fmt.Sprintf("Ошибка %s не является временной, пропускаем повторы", name),
					String("component", name),
				)
			}
			break
		}
		
		if logger != nil {
			logger.Warn(ctx, fmt.Sprintf("Попытка %d/%d для %s не удалась", attempt+1, maxRetries+1, name),
				String("component", name),
				Int("attempt", attempt+1),
				Int("max_attempts", maxRetries+1),
				Err(err),
			)
		}
	}
	
	return lastErr
}

// WrapWithPanicRecovery оборачивает функцию в panic recovery
func WrapWithPanicRecovery(name string, fn func(), logger StructuredLogger) func() {
	return func() {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				
				if logger != nil {
					logger.Error(context.Background(), fmt.Sprintf("PANIC в %s восстановлен", name),
						String("component", name),
						Any("panic_value", r),
						String("stack_trace", string(stack)),
					)
				}
			}
		}()
		
		fn()
	}
}

// RecoveryMiddleware middleware для добавления recovery к обработчикам
type RecoveryMiddleware struct {
	handler RecoveryHandler
	logger  StructuredLogger
}

// NewRecoveryMiddleware создает новый recovery middleware
func NewRecoveryMiddleware(handler RecoveryHandler, logger StructuredLogger) *RecoveryMiddleware {
	return &RecoveryMiddleware{
		handler: handler,
		logger:  logger,
	}
}

// WrapDialogHandler оборачивает обработчик диалогов
func (rm *RecoveryMiddleware) WrapDialogHandler(name string, handler func(*Dialog)) func(*Dialog) {
	return func(dialog *Dialog) {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				
				ctx := context.Background()
				if dialog != nil {
					// Добавляем контекст диалога
					ctx = context.WithValue(ctx, "call_id", dialog.callID)
					ctx = context.WithValue(ctx, "dialog_id", dialog.key.String())
				}
				
				rm.handler.HandlePanic(ctx, r, stack, name)
			}
		}()
		
		handler(dialog)
	}
}

// WrapTransactionHandler оборачивает обработчик транзакций
func (rm *RecoveryMiddleware) WrapTransactionHandler(name string, handler func(*TransactionAdapter)) func(*TransactionAdapter) {
	return func(tx *TransactionAdapter) {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				
				ctx := context.Background()
				if tx != nil {
					ctx = context.WithValue(ctx, "transaction_id", tx.ID())
				}
				
				rm.handler.HandlePanic(ctx, r, stack, name)
			}
		}()
		
		handler(tx)
	}
}

// GetGoroutineID получает ID текущей горутины (для отладки)
func GetGoroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	// Парсим "goroutine 123 [running]:"
	idField := string(buf[:n])
	var id uint64
	fmt.Sscanf(idField, "goroutine %d ", &id)
	return id
}