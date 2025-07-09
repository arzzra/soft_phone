# План реализации P1 исправлений для пакета dialog

## Приоритетность задач

### 1. Критические исправления безопасности (Приоритет: ВЫСШИЙ)

#### 1.1 Валидация входных данных и защита от инъекций

**Файлы для изменения:**
- `pkg/dialog/security.go` (создать новый)
- `pkg/dialog/dialog.go` 
- `pkg/dialog/manager.go`
- `pkg/dialog/refer.go`

**Конкретные задачи:**

1. **Создать модуль безопасности** (`security.go`):
```go
package dialog

import (
    "fmt"
    "net"
    "strings"
    "sync"
    "time"
)

// Константы безопасности
const (
    MaxHeaderLength = 8192  // Максимальная длина заголовка
    MaxBodyLength   = 65536 // Максимальная длина тела
    MaxDialogsPerIP = 100   // Максимум диалогов с одного IP
    MaxURILength    = 2048  // Максимальная длина URI
)

// SecurityValidator проверяет безопасность входных данных
type SecurityValidator struct {
    ipCounter   map[string]*ipInfo
    ipMutex     sync.RWMutex
    cleanupTime time.Duration
}

type ipInfo struct {
    count      int
    lastAccess time.Time
}

func NewSecurityValidator() *SecurityValidator {
    sv := &SecurityValidator{
        ipCounter:   make(map[string]*ipInfo),
        cleanupTime: 5 * time.Minute,
    }
    go sv.cleanupLoop()
    return sv
}

// ValidateHeader проверяет заголовок на безопасность
func (sv *SecurityValidator) ValidateHeader(name, value string) error {
    if len(value) > MaxHeaderLength {
        return fmt.Errorf("header %s too long: %d bytes", name, len(value))
    }
    
    // Проверка на опасные символы
    if strings.ContainsAny(value, "\r\n\x00") {
        return fmt.Errorf("invalid characters in header %s", name)
    }
    
    // Специальная проверка для URI заголовков
    if name == "Refer-To" || name == "Contact" || name == "From" || name == "To" {
        if err := sv.ValidateURI(value); err != nil {
            return fmt.Errorf("invalid URI in %s: %w", name, err)
        }
    }
    
    return nil
}

// ValidateURI проверяет URI на безопасность
func (sv *SecurityValidator) ValidateURI(uri string) error {
    if len(uri) > MaxURILength {
        return fmt.Errorf("URI too long: %d bytes", len(uri))
    }
    
    // Проверка на SQL/Command injection паттерны
    dangerousPatterns := []string{
        "';", "--", "/*", "*/", "xp_", "sp_",
        "&&", "||", "|", ";", "`", "$(",
    }
    
    uriLower := strings.ToLower(uri)
    for _, pattern := range dangerousPatterns {
        if strings.Contains(uriLower, pattern) {
            return fmt.Errorf("potentially dangerous pattern in URI: %s", pattern)
        }
    }
    
    return nil
}

// CheckRateLimit проверяет лимиты для IP
func (sv *SecurityValidator) CheckRateLimit(remoteAddr string) error {
    ip, _, err := net.SplitHostPort(remoteAddr)
    if err != nil {
        ip = remoteAddr // Если не удалось разделить, используем как есть
    }
    
    sv.ipMutex.Lock()
    defer sv.ipMutex.Unlock()
    
    info, exists := sv.ipCounter[ip]
    if !exists {
        sv.ipCounter[ip] = &ipInfo{
            count:      1,
            lastAccess: time.Now(),
        }
        return nil
    }
    
    info.lastAccess = time.Now()
    if info.count >= MaxDialogsPerIP {
        return fmt.Errorf("rate limit exceeded for IP %s: %d dialogs", ip, info.count)
    }
    
    info.count++
    return nil
}

// ReleaseRateLimit уменьшает счетчик для IP
func (sv *SecurityValidator) ReleaseRateLimit(remoteAddr string) {
    ip, _, err := net.SplitHostPort(remoteAddr)
    if err != nil {
        ip = remoteAddr
    }
    
    sv.ipMutex.Lock()
    defer sv.ipMutex.Unlock()
    
    if info, exists := sv.ipCounter[ip]; exists && info.count > 0 {
        info.count--
        if info.count == 0 {
            delete(sv.ipCounter, ip)
        }
    }
}

// cleanupLoop периодически очищает устаревшие записи
func (sv *SecurityValidator) cleanupLoop() {
    ticker := time.NewTicker(sv.cleanupTime)
    defer ticker.Stop()
    
    for range ticker.C {
        sv.ipMutex.Lock()
        now := time.Now()
        for ip, info := range sv.ipCounter {
            if now.Sub(info.lastAccess) > sv.cleanupTime && info.count == 0 {
                delete(sv.ipCounter, ip)
            }
        }
        sv.ipMutex.Unlock()
    }
}

// ValidateBody проверяет тело сообщения
func (sv *SecurityValidator) ValidateBody(body []byte, contentType string) error {
    if len(body) > MaxBodyLength {
        return fmt.Errorf("body too large: %d bytes", len(body))
    }
    
    // Дополнительные проверки в зависимости от content-type
    if strings.Contains(contentType, "application/sdp") {
        return sv.validateSDP(body)
    }
    
    return nil
}

// validateSDP проверяет SDP на безопасность
func (sv *SecurityValidator) validateSDP(body []byte) error {
    // Базовая проверка структуры SDP
    sdpStr := string(body)
    if !strings.Contains(sdpStr, "v=0") {
        return fmt.Errorf("invalid SDP: missing version")
    }
    
    // Проверка на слишком большое количество медиа-линий
    mediaCount := strings.Count(sdpStr, "\nm=")
    if mediaCount > 10 {
        return fmt.Errorf("too many media lines in SDP: %d", mediaCount)
    }
    
    return nil
}
```

2. **Обновить dialog.go для использования валидации**:
   - Добавить поле `validator *SecurityValidator` в структуру Dialog
   - Добавить валидацию во все методы обработки запросов
   - Пример для handleREFER:

```go
// В структуру Dialog добавить:
type Dialog struct {
    // ... существующие поля
    validator *SecurityValidator
}

// Обновить handleREFER:
func (d *Dialog) handleREFER(req *sip.Request, tx sip.ServerTransaction) error {
    // Валидация заголовка Refer-To
    referToHeader := req.GetHeader("Refer-To")
    if referToHeader == nil {
        res := sip.NewResponseFromRequest(req, sip.StatusBadRequest, "Missing Refer-To header", nil)
        return tx.Respond(res)
    }
    
    // Проверка безопасности
    if err := d.validator.ValidateHeader("Refer-To", referToHeader.Value()); err != nil {
        res := sip.NewResponseFromRequest(req, sip.StatusBadRequest, fmt.Sprintf("Invalid Refer-To: %v", err), nil)
        return tx.Respond(res)
    }
    
    // ... остальная логика
}
```

3. **Обновить DialogManager для rate limiting**:
```go
// В DialogManager добавить:
type DialogManager struct {
    // ... существующие поля
    validator *SecurityValidator
}

// В CreateServerDialog добавить:
func (dm *DialogManager) CreateServerDialog(req *sip.Request, tx sip.ServerTransaction) (IDialog, error) {
    // Проверка rate limit
    remoteAddr := tx.Destination()
    if err := dm.validator.CheckRateLimit(remoteAddr); err != nil {
        return nil, err
    }
    
    // ... создание диалога
    
    // При ошибке освобождаем лимит
    defer func() {
        if err != nil {
            dm.validator.ReleaseRateLimit(remoteAddr)
        }
    }()
    
    // ... остальная логика
}
```

#### 1.2 Усиленная валидация parseReferTo и parseReplaces

**Файл:** `pkg/dialog/refer.go`

```go
// Обновить parseReferTo:
func parseReferTo(referTo string) (*sip.Uri, map[string]string, error) {
    // Добавить проверку длины
    if len(referTo) > MaxURILength {
        return nil, nil, fmt.Errorf("Refer-To too long: %d bytes", len(referTo))
    }
    
    // Удалить пробелы и проверить формат
    referTo = strings.TrimSpace(referTo)
    if referTo == "" {
        return nil, nil, fmt.Errorf("empty Refer-To")
    }
    
    // Проверка на опасные символы
    if strings.ContainsAny(referTo, "\r\n\x00") {
        return nil, nil, fmt.Errorf("invalid characters in Refer-To")
    }
    
    // ... существующая логика парсинга с дополнительными проверками
}

// Обновить parseReplaces:
func parseReplaces(replaces string) (callID, toTag, fromTag string, err error) {
    // Проверка длины
    if len(replaces) > 512 { // Replaces не должен быть слишком длинным
        return "", "", "", fmt.Errorf("Replaces header too long")
    }
    
    // Проверка формата
    parts := strings.Split(replaces, ";")
    if len(parts) < 1 || len(parts) > 3 {
        return "", "", "", fmt.Errorf("invalid Replaces format")
    }
    
    // Валидация Call-ID
    callID = strings.TrimSpace(parts[0])
    if callID == "" || strings.ContainsAny(callID, "\r\n\x00<>\"") {
        return "", "", "", fmt.Errorf("invalid Call-ID in Replaces")
    }
    
    // ... остальная логика с проверками
}
```

### 2. Улучшение обработки ошибок (Приоритет: ВЫСОКИЙ)

#### 2.1 Создание системы логирования

**Файлы:**
- `pkg/dialog/logger.go` (создать новый)
- Обновить все файлы где игнорируются ошибки

```go
// logger.go
package dialog

import (
    "context"
    "fmt"
    "log/slog"
    "os"
)

// Logger интерфейс для логирования
type Logger interface {
    Error(ctx context.Context, msg string, args ...any)
    Warn(ctx context.Context, msg string, args ...any)
    Info(ctx context.Context, msg string, args ...any)
    Debug(ctx context.Context, msg string, args ...any)
    With(args ...any) Logger
}

// slogLogger обертка над slog
type slogLogger struct {
    logger *slog.Logger
}

// NewDefaultLogger создает логгер по умолчанию
func NewDefaultLogger() Logger {
    opts := &slog.HandlerOptions{
        Level: slog.LevelInfo,
    }
    handler := slog.NewJSONHandler(os.Stderr, opts)
    return &slogLogger{
        logger: slog.New(handler).With("component", "dialog"),
    }
}

func (l *slogLogger) Error(ctx context.Context, msg string, args ...any) {
    l.logger.ErrorContext(ctx, msg, args...)
}

func (l *slogLogger) Warn(ctx context.Context, msg string, args ...any) {
    l.logger.WarnContext(ctx, msg, args...)
}

func (l *slogLogger) Info(ctx context.Context, msg string, args ...any) {
    l.logger.InfoContext(ctx, msg, args...)
}

func (l *slogLogger) Debug(ctx context.Context, msg string, args ...any) {
    l.logger.DebugContext(ctx, msg, args...)
}

func (l *slogLogger) With(args ...any) Logger {
    return &slogLogger{
        logger: l.logger.With(args...),
    }
}
```

#### 2.2 Замена игнорируемых ошибок

**Конкретные места для исправления:**

1. **dialog.go, строка 665** - ProcessRouteHeaders:
```go
// Было:
_ = d.headerProcessor.ProcessRouteHeaders(req, d.routeSet)

// Стало:
if err := d.headerProcessor.ProcessRouteHeaders(req, d.routeSet); err != nil {
    d.logger.Warn(context.Background(), "failed to process route headers",
        "error", err,
        "dialog_id", d.ID(),
        "method", req.Method)
    // Продолжаем выполнение, так как это не критично
}
```

2. **dialog.go, строки 799-850** - обработка в горутинах:
```go
// Добавить обработку ошибок в горутине handleREFER:
go func() {
    defer func() {
        if r := recover(); r != nil {
            d.logger.Error(ctx, "panic in REFER handler",
                "panic", r,
                "dialog_id", d.ID())
        }
    }()
    
    // Отправляем NOTIFY
    if err := subscription.SendNotify(ctx, "SIP/2.0 100 Trying"); err != nil {
        d.logger.Error(ctx, "failed to send initial NOTIFY",
            "error", err,
            "refer_to", referToURI.String())
        return
    }
    
    // ... остальная логика с логированием ошибок
}()
```

#### 2.3 Система повторных попыток

**Файл:** `pkg/dialog/retry.go` (создать новый)

```go
package dialog

import (
    "context"
    "errors"
    "fmt"
    "time"
)

// RetryConfig конфигурация повторных попыток
type RetryConfig struct {
    MaxAttempts     int
    InitialInterval time.Duration
    MaxInterval     time.Duration
    Multiplier      float64
}

// DefaultRetryConfig конфигурация по умолчанию
var DefaultRetryConfig = RetryConfig{
    MaxAttempts:     3,
    InitialInterval: 100 * time.Millisecond,
    MaxInterval:     5 * time.Second,
    Multiplier:      2.0,
}

// RetryableError ошибка которую можно повторить
type RetryableError struct {
    Err error
}

func (e RetryableError) Error() string {
    return fmt.Sprintf("retryable: %v", e.Err)
}

// IsRetryable проверяет можно ли повторить ошибку
func IsRetryable(err error) bool {
    if err == nil {
        return false
    }
    
    var retryable RetryableError
    if errors.As(err, &retryable) {
        return true
    }
    
    // Проверяем временные сетевые ошибки
    return errors.Is(err, context.DeadlineExceeded) ||
           errors.Is(err, context.Canceled) ||
           isTemporaryNetError(err)
}

// RetryWithBackoff выполняет функцию с повторными попытками
func RetryWithBackoff(ctx context.Context, config RetryConfig, logger Logger, operation string, fn func() error) error {
    var lastErr error
    interval := config.InitialInterval
    
    for attempt := 0; attempt < config.MaxAttempts; attempt++ {
        if attempt > 0 {
            logger.Debug(ctx, "retrying operation",
                "operation", operation,
                "attempt", attempt+1,
                "max_attempts", config.MaxAttempts,
                "interval", interval)
        }
        
        lastErr = fn()
        if lastErr == nil {
            return nil
        }
        
        if !IsRetryable(lastErr) {
            return lastErr
        }
        
        // Последняя попытка - не ждем
        if attempt == config.MaxAttempts-1 {
            break
        }
        
        select {
        case <-ctx.Done():
            return fmt.Errorf("retry cancelled: %w", ctx.Err())
        case <-time.After(interval):
        }
        
        // Увеличиваем интервал
        interval = time.Duration(float64(interval) * config.Multiplier)
        if interval > config.MaxInterval {
            interval = config.MaxInterval
        }
    }
    
    return fmt.Errorf("failed after %d attempts: %w", config.MaxAttempts, lastErr)
}

// isTemporaryNetError проверяет временные сетевые ошибки
func isTemporaryNetError(err error) bool {
    // Здесь можно добавить проверку специфичных сетевых ошибок
    return false
}
```

### 3. Оптимизация производительности (Приоритет: СРЕДНИЙ)

#### 3.1 Добавление индексов в DialogManager

**Файл:** `pkg/dialog/manager.go`

```go
// Обновить структуру DialogManager:
type DialogManager struct {
    mu       sync.RWMutex
    dialogs  map[string]IDialog
    uasuac   *UASUAC
    logger   Logger
    
    // Индексы для быстрого поиска
    callIDIndex    map[string][]string        // callID -> []dialogID
    tagIndex       map[string]string          // "localTag:remoteTag" -> dialogID
    remoteTagIndex map[string][]string        // remoteTag -> []dialogID
    
    // Производительность
    workerPool *WorkerPool
    validator  *SecurityValidator
}

// Обновить методы для поддержки индексов:
func (dm *DialogManager) RegisterDialog(dialog IDialog) error {
    dm.mu.Lock()
    defer dm.mu.Unlock()
    
    dialogID := dialog.ID()
    if _, exists := dm.dialogs[dialogID]; exists {
        return fmt.Errorf("dialog already exists: %s", dialogID)
    }
    
    dm.dialogs[dialogID] = dialog
    
    // Обновляем индексы
    dm.updateIndexesLocked(dialog, true)
    
    dm.logger.Info(context.Background(), "dialog registered",
        "dialog_id", dialogID,
        "call_id", dialog.CallID().Value())
    
    return nil
}

// updateIndexesLocked обновляет индексы (вызывать под блокировкой)
func (dm *DialogManager) updateIndexesLocked(dialog IDialog, add bool) {
    dialogID := dialog.ID()
    callID := dialog.CallID().Value()
    localTag := dialog.LocalTag()
    remoteTag := dialog.RemoteTag()
    
    if add {
        // Добавляем в индексы
        dm.callIDIndex[callID] = append(dm.callIDIndex[callID], dialogID)
        
        if localTag != "" && remoteTag != "" {
            tagKey := fmt.Sprintf("%s:%s", localTag, remoteTag)
            dm.tagIndex[tagKey] = dialogID
        }
        
        if remoteTag != "" {
            dm.remoteTagIndex[remoteTag] = append(dm.remoteTagIndex[remoteTag], dialogID)
        }
    } else {
        // Удаляем из индексов
        dm.removeFromSlice(&dm.callIDIndex[callID], dialogID)
        if len(dm.callIDIndex[callID]) == 0 {
            delete(dm.callIDIndex, callID)
        }
        
        if localTag != "" && remoteTag != "" {
            tagKey := fmt.Sprintf("%s:%s", localTag, remoteTag)
            delete(dm.tagIndex, tagKey)
        }
        
        if remoteTag != "" {
            dm.removeFromSlice(&dm.remoteTagIndex[remoteTag], dialogID)
            if len(dm.remoteTagIndex[remoteTag]) == 0 {
                delete(dm.remoteTagIndex, remoteTag)
            }
        }
    }
}

// removeFromSlice удаляет элемент из слайса
func (dm *DialogManager) removeFromSlice(slice *[]string, value string) {
    for i, v := range *slice {
        if v == value {
            *slice = append((*slice)[:i], (*slice)[i+1:]...)
            return
        }
    }
}

// Оптимизированный GetDialogByRequest
func (dm *DialogManager) GetDialogByRequest(req *sip.Request) (IDialog, error) {
    dm.mu.RLock()
    defer dm.mu.RUnlock()
    
    callID := req.CallID()
    if callID == nil {
        return nil, fmt.Errorf("missing Call-ID header")
    }
    
    fromTag, _ := req.From().Params.Get("tag")
    toTag, _ := req.To().Params.Get("tag")
    
    // Быстрый поиск по индексу тегов
    if fromTag != "" && toTag != "" {
        // Пробуем оба варианта (UAC и UAS)
        if dialogID, exists := dm.tagIndex[fmt.Sprintf("%s:%s", fromTag, toTag)]; exists {
            return dm.dialogs[dialogID], nil
        }
        if dialogID, exists := dm.tagIndex[fmt.Sprintf("%s:%s", toTag, fromTag)]; exists {
            return dm.dialogs[dialogID], nil
        }
    }
    
    // Если не нашли по тегам, ищем по Call-ID
    dialogIDs, exists := dm.callIDIndex[callID.Value()]
    if !exists {
        return nil, fmt.Errorf("dialog not found for Call-ID: %s", callID.Value())
    }
    
    // Проверяем каждый диалог
    for _, dialogID := range dialogIDs {
        dialog := dm.dialogs[dialogID]
        if dialog != nil && dm.isRequestForDialog(req, dialog) {
            return dialog, nil
        }
    }
    
    return nil, fmt.Errorf("dialog not found")
}
```

#### 3.2 Пул воркеров для обработки событий

**Файл:** `pkg/dialog/worker_pool.go` (создать новый)

```go
package dialog

import (
    "context"
    "runtime"
    "sync"
    "sync/atomic"
)

// Task представляет задачу для выполнения
type Task struct {
    Name     string
    Priority int
    Execute  func() error
}

// WorkerPool пул воркеров для обработки задач
type WorkerPool struct {
    workers    int
    maxQueue   int
    taskQueue  chan *Task
    wg         sync.WaitGroup
    ctx        context.Context
    cancel     context.CancelFunc
    logger     Logger
    
    // Метрики
    processed  atomic.Uint64
    failed     atomic.Uint64
    queueSize  atomic.Int32
}

// NewWorkerPool создает новый пул воркеров
func NewWorkerPool(workers int, queueSize int, logger Logger) *WorkerPool {
    if workers <= 0 {
        workers = runtime.NumCPU()
    }
    if queueSize <= 0 {
        queueSize = workers * 10
    }
    
    ctx, cancel := context.WithCancel(context.Background())
    pool := &WorkerPool{
        workers:   workers,
        maxQueue:  queueSize,
        taskQueue: make(chan *Task, queueSize),
        ctx:       ctx,
        cancel:    cancel,
        logger:    logger.With("component", "worker_pool"),
    }
    
    pool.start()
    return pool
}

// start запускает воркеров
func (p *WorkerPool) start() {
    for i := 0; i < p.workers; i++ {
        p.wg.Add(1)
        go p.worker(i)
    }
    
    p.logger.Info(p.ctx, "worker pool started",
        "workers", p.workers,
        "queue_size", p.maxQueue)
}

// worker обрабатывает задачи
func (p *WorkerPool) worker(id int) {
    defer p.wg.Done()
    
    logger := p.logger.With("worker_id", id)
    logger.Debug(p.ctx, "worker started")
    
    for {
        select {
        case <-p.ctx.Done():
            logger.Debug(p.ctx, "worker stopped")
            return
            
        case task, ok := <-p.taskQueue:
            if !ok {
                return
            }
            
            p.queueSize.Add(-1)
            p.processTask(task, logger)
        }
    }
}

// processTask обрабатывает одну задачу
func (p *WorkerPool) processTask(task *Task, logger Logger) {
    defer func() {
        if r := recover(); r != nil {
            p.failed.Add(1)
            logger.Error(p.ctx, "panic in task execution",
                "task", task.Name,
                "panic", r)
        }
    }()
    
    if err := task.Execute(); err != nil {
        p.failed.Add(1)
        logger.Error(p.ctx, "task execution failed",
            "task", task.Name,
            "error", err)
    } else {
        p.processed.Add(1)
    }
}

// Submit отправляет задачу в пул
func (p *WorkerPool) Submit(task *Task) error {
    select {
    case <-p.ctx.Done():
        return context.Canceled
        
    case p.taskQueue <- task:
        p.queueSize.Add(1)
        return nil
        
    default:
        return fmt.Errorf("worker pool queue is full")
    }
}

// SubmitWithTimeout отправляет задачу с таймаутом
func (p *WorkerPool) SubmitWithTimeout(task *Task, timeout time.Duration) error {
    ctx, cancel := context.WithTimeout(p.ctx, timeout)
    defer cancel()
    
    select {
    case <-ctx.Done():
        return ctx.Err()
        
    case p.taskQueue <- task:
        p.queueSize.Add(1)
        return nil
    }
}

// Stop останавливает пул воркеров
func (p *WorkerPool) Stop() {
    p.logger.Info(p.ctx, "stopping worker pool")
    
    p.cancel()
    close(p.taskQueue)
    p.wg.Wait()
    
    p.logger.Info(p.ctx, "worker pool stopped",
        "processed", p.processed.Load(),
        "failed", p.failed.Load())
}

// Stats возвращает статистику пула
func (p *WorkerPool) Stats() (processed, failed uint64, queueSize int32) {
    return p.processed.Load(), p.failed.Load(), p.queueSize.Load()
}
```

### 4. Интеграция всех компонентов

#### 4.1 Обновление конструкторов

**Файл:** `pkg/dialog/dialog.go`

```go
// Обновить NewDialog:
func NewDialog(callID sip.CallIDHeader, localTag, remoteTag string, isServer bool, uasuac *UASUAC) *Dialog {
    ctx, cancel := context.WithCancel(context.Background())
    
    logger := NewDefaultLogger()
    if uasuac != nil && uasuac.logger != nil {
        logger = uasuac.logger
    }
    
    d := &Dialog{
        id:              generateDialogID(callID, localTag, remoteTag, isServer),
        callID:          callID,
        localTag:        localTag,
        remoteTag:       remoteTag,
        isServer:        isServer,
        state:           StateNone,
        ctx:             ctx,
        cancel:          cancel,
        createdAt:       time.Now(),
        lastActivity:    time.Now(),
        uasuac:          uasuac,
        headerProcessor: NewHeaderProcessor(),
        validator:       NewSecurityValidator(),
        logger:          logger.With("dialog_id", d.id),
        tasks:           make(map[string]*backgroundTask),
    }
    
    d.initStateMachine()
    return d
}
```

**Файл:** `pkg/dialog/manager.go`

```go
// Обновить NewDialogManager:
func NewDialogManager(uasuac *UASUAC) *DialogManager {
    logger := NewDefaultLogger()
    if uasuac != nil && uasuac.logger != nil {
        logger = uasuac.logger
    }
    
    dm := &DialogManager{
        dialogs:        make(map[string]IDialog),
        callIDIndex:    make(map[string][]string),
        tagIndex:       make(map[string]string),
        remoteTagIndex: make(map[string][]string),
        uasuac:         uasuac,
        logger:         logger.With("component", "dialog_manager"),
        validator:      NewSecurityValidator(),
        workerPool:     NewWorkerPool(10, 100, logger),
    }
    
    // Запускаем периодическую очистку
    go dm.periodicCleanup()
    
    return dm
}

// periodicCleanup периодически очищает завершенные диалоги
func (dm *DialogManager) periodicCleanup() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            dm.CleanupTerminated()
        }
    }
}
```

### 5. Тестирование и валидация

#### 5.1 Unit тесты для безопасности

**Файл:** `pkg/dialog/security_test.go` (создать новый)

```go
package dialog

import (
    "testing"
    "strings"
)

func TestSecurityValidator_ValidateHeader(t *testing.T) {
    sv := NewSecurityValidator()
    
    tests := []struct {
        name    string
        header  string
        value   string
        wantErr bool
    }{
        {"valid header", "From", "<sip:user@example.com>", false},
        {"too long header", "From", strings.Repeat("x", MaxHeaderLength+1), true},
        {"injection attempt", "Refer-To", "sip:user@example.com>;tag=123\r\nInjected: header", true},
        {"sql injection", "Refer-To", "sip:user'; DROP TABLE users;--@example.com", true},
        {"command injection", "Contact", "sip:user$(whoami)@example.com", true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := sv.ValidateHeader(tt.header, tt.value)
            if (err != nil) != tt.wantErr {
                t.Errorf("ValidateHeader() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}

func TestSecurityValidator_RateLimit(t *testing.T) {
    sv := NewSecurityValidator()
    
    // Тестируем rate limiting
    ip := "192.168.1.1:5060"
    
    // Должны пройти первые MaxDialogsPerIP запросов
    for i := 0; i < MaxDialogsPerIP; i++ {
        if err := sv.CheckRateLimit(ip); err != nil {
            t.Errorf("CheckRateLimit() failed at %d: %v", i, err)
        }
    }
    
    // Следующий должен быть отклонен
    if err := sv.CheckRateLimit(ip); err == nil {
        t.Error("CheckRateLimit() should have failed after limit")
    }
    
    // После освобождения должен пройти
    sv.ReleaseRateLimit(ip)
    if err := sv.CheckRateLimit(ip); err != nil {
        t.Error("CheckRateLimit() failed after release:", err)
    }
}
```

#### 5.2 Race condition тесты

**Файл:** `pkg/dialog/dialog_race_test.go` (создать новый)

```go
package dialog

import (
    "context"
    "sync"
    "testing"
    "time"
)

func TestDialog_RaceConditions(t *testing.T) {
    // Создаем тестовый диалог
    callID := sip.CallIDHeader{Value: func() string { return "test-call-id" }}
    d := NewDialog(callID, "local-tag", "remote-tag", false, nil)
    
    // Запускаем конкурентные операции
    var wg sync.WaitGroup
    ctx := context.Background()
    
    // Горутина 1: обновление ID
    wg.Add(1)
    go func() {
        defer wg.Done()
        for i := 0; i < 100; i++ {
            d.updateDialogID()
            time.Sleep(time.Microsecond)
        }
    }()
    
    // Горутина 2: чтение состояния
    wg.Add(1)
    go func() {
        defer wg.Done()
        for i := 0; i < 100; i++ {
            _ = d.State()
            _ = d.ID()
            time.Sleep(time.Microsecond)
        }
    }()
    
    // Горутина 3: применение заголовков
    wg.Add(1)
    go func() {
        defer wg.Done()
        req := &sip.Request{}
        for i := 0; i < 100; i++ {
            d.applyDialogHeaders(req)
            time.Sleep(time.Microsecond)
        }
    }()
    
    // Горутина 4: изменение состояния
    wg.Add(1)
    go func() {
        defer wg.Done()
        for i := 0; i < 50; i++ {
            _ = d.stateMachine.Event(ctx, "provisional")
            _ = d.stateMachine.Event(ctx, "confirm")
            time.Sleep(time.Millisecond)
        }
    }()
    
    wg.Wait()
    
    // Проверяем что диалог в корректном состоянии
    if d.State() == StateNone {
        t.Error("Dialog in invalid state after concurrent operations")
    }
}
```

### 6. Последовательность реализации

1. **День 1-2: Безопасность**
   - Создать security.go с валидаторами
   - Обновить parseReferTo и parseReplaces
   - Добавить валидацию во все обработчики запросов
   - Добавить rate limiting в DialogManager

2. **День 3-4: Логирование и обработка ошибок**
   - Создать logger.go
   - Создать retry.go
   - Заменить все игнорируемые ошибки на логирование
   - Добавить retry механизм для критических операций

3. **День 5-6: Производительность**
   - Создать worker_pool.go
   - Добавить индексы в DialogManager
   - Оптимизировать поиск диалогов
   - Реализовать пул воркеров для событий

4. **День 7: Интеграция и тестирование**
   - Обновить конструкторы для использования новых компонентов
   - Создать unit тесты для безопасности
   - Создать race condition тесты
   - Запустить полное тестирование с `go test -race`

5. **День 8: Финальная проверка**
   - Запустить линтер `make lint`
   - Исправить все предупреждения
   - Провести нагрузочное тестирование
   - Подготовить документацию

### 7. Критерии завершения

1. **Безопасность:**
   - ✓ Все входные данные валидируются
   - ✓ Rate limiting работает
   - ✓ Нет уязвимостей к инъекциям

2. **Обработка ошибок:**
   - ✓ Все ошибки логируются
   - ✓ Критические операции имеют retry
   - ✓ Нет паник в production коде

3. **Производительность:**
   - ✓ O(1) поиск диалогов по ключам
   - ✓ Пул воркеров обрабатывает события
   - ✓ Нет деградации под нагрузкой

4. **Качество кода:**
   - ✓ Проходит `go test -race`
   - ✓ Проходит `make lint`
   - ✓ 80%+ покрытие тестами