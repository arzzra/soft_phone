# План исправления критических проблем в пакете dialog

## Анализ обнаруженных проблем

### P0 - Критические проблемы

#### 1. Race Conditions (Гонки данных)
**Проблемы:**
- `handleREFER` (строка 687): доступ к полям диалога без блокировки перед созданием горутины
- `updateDialogID` (строка 141): вызывается без защиты мьютекса из нескольких мест
- `applyDialogHeaders` (строка 583): изменяет `localSeq` без блокировки

**Места вызова без защиты:**
```go
// dialog.go:844 - внутри обработчика ответов
d.updateDialogID() // вызывается без блокировки

// dialog.go:602 - инкремент без блокировки
d.localSeq++
```

#### 2. Утечки ресурсов
**Проблемы:**
- Горутина в `handleREFER` (строка 735) не имеет механизма отмены
- Горутина для обновления ID диалога (строка 848) запускается без контроля
- Отсутствует метод `Close()` в интерфейсе `IDialog` для очистки ресурсов
- `ReferSubscription` имеет контекст и канал, но не всегда корректно закрывается

#### 3. Null Pointer Dereference
**Проблемы:**
- В `handleREFER` нет проверки на nil для `d.uasuac` перед использованием
- Множественные места где игнорируются ошибки, которые могут привести к паникам

### P1 - Важные проблемы

#### 1. Безопасность
**Проблемы:**
- Недостаточная валидация входных данных в `parseReferTo`, `parseReplaces`
- Отсутствие лимитов на размер заголовков и тела сообщений
- Нет защиты от DoS атак через создание большого количества диалогов

#### 2. Обработка ошибок
**Проблемы:**
- Игнорирование ошибок в критических местах (строки 609, 850, 740, 741, 751, 757)
- Отсутствие логирования ошибок
- Нет механизма повторных попыток для временных сбоев

#### 3. Производительность
**Проблемы:**
- Линейный поиск диалогов в `DialogManager` (O(n) сложность)
- Отсутствие индексов для быстрого поиска по составным ключам
- Нет пула горутин для обработки событий

## Детальный план исправлений

### Фаза 1: Исправление критических Race Conditions (1-2 дня)

#### Задача 1.1: Защита updateDialogID
```go
// Изменить updateDialogID на потокобезопасную версию
func (d *Dialog) updateDialogID() {
    d.mu.Lock()
    defer d.mu.Unlock()
    
    // существующая логика
    if d.isServer {
        d.id = fmt.Sprintf("%s;from-tag=%s;to-tag=%s", d.callID.Value(), d.remoteTag, d.localTag)
    } else {
        d.id = fmt.Sprintf("%s;from-tag=%s;to-tag=%s", d.callID.Value(), d.localTag, d.remoteTag)
    }
}

// Создать безопасную версию для внешних вызовов
func (d *Dialog) safeUpdateDialogID() {
    oldID := d.ID() // уже защищено мьютексом
    d.updateDialogID()
    newID := d.ID()
    
    if oldID != newID && d.uasuac != nil && d.uasuac.dialogManager != nil {
        // Обновление в менеджере теперь синхронное
        _ = d.uasuac.dialogManager.UpdateDialogID(oldID, newID, d)
    }
}
```

#### Задача 1.2: Защита applyDialogHeaders
```go
func (d *Dialog) applyDialogHeaders(req *sip.Request) {
    d.mu.Lock()
    defer d.mu.Unlock()
    
    // Копируем необходимые данные под защитой мьютекса
    localURI := d.localURI
    remoteURI := d.remoteURI
    localTag := d.localTag
    remoteTag := d.remoteTag
    callID := d.callID
    localTarget := d.localTarget
    routeSet := make([]sip.RouteHeader, len(d.routeSet))
    copy(routeSet, d.routeSet)
    
    // Инкрементируем CSeq под защитой
    d.localSeq++
    localSeq := d.localSeq
    
    // Разблокируем перед применением заголовков
    d.mu.Unlock()
    
    // Применяем заголовки без блокировки
    fromValue := fmt.Sprintf("<%s>", localURI.String())
    if localTag != "" {
        fromValue += fmt.Sprintf(";tag=%s", localTag)
    }
    req.ReplaceHeader(sip.NewHeader("From", fromValue))
    
    // ... остальная логика применения заголовков
    
    d.mu.Lock() // Восстанавливаем блокировку для defer
}
```

#### Задача 1.3: Исправление handleREFER
```go
func (d *Dialog) handleREFER(req *sip.Request, tx sip.ServerTransaction) error {
    // Копируем необходимые данные под защитой мьютекса
    d.mu.RLock()
    dialogCtx := d.ctx
    uasuac := d.uasuac
    referHandler := d.referHandler
    d.mu.RUnlock()
    
    // Проверяем что диалог не завершен
    if dialogCtx.Err() != nil {
        res := sip.NewResponseFromRequest(req, sip.StatusCallDoesNotExist, "Dialog terminated", nil)
        return tx.Respond(res)
    }
    
    // Проверяем наличие uasuac
    if uasuac == nil {
        res := sip.NewResponseFromRequest(req, sip.StatusInternalServerError, "No UA configured", nil)
        return tx.Respond(res)
    }
    
    // ... остальная логика обработки REFER
    
    // Запускаем обработку в контролируемой горутине
    go func() {
        defer func() {
            if r := recover(); r != nil {
                // Логируем панику
                fmt.Printf("Panic in REFER handler: %v\n", r)
            }
        }()
        
        // Используем контекст диалога для отмены
        select {
        case <-dialogCtx.Done():
            return
        default:
        }
        
        // ... обработка REFER
    }()
    
    return nil
}
```

### Фаза 2: Устранение утечек ресурсов (1-2 дня)

#### Задача 2.1: Добавление метода Close в IDialog
```go
// В interfaces.go добавить:
type IDialog interface {
    // ... существующие методы
    
    // Close освобождает все ресурсы диалога
    Close() error
}

// В dialog.go реализовать:
func (d *Dialog) Close() error {
    d.mu.Lock()
    defer d.mu.Unlock()
    
    // Проверяем что еще не закрыто
    if d.ctx.Err() != nil {
        return nil
    }
    
    // Отменяем контекст
    d.cancel()
    
    // Переводим в завершенное состояние
    _ = d.stateMachine.Event(context.Background(), "terminated")
    
    // Очищаем обработчики для освобождения памяти
    d.stateChangeHandler = nil
    d.bodyHandler = nil
    d.requestHandler = nil
    d.referHandler = nil
    
    return nil
}
```

#### Задача 2.2: Управление горутинами
```go
// Создать структуру для управления фоновыми задачами
type backgroundTask struct {
    cancel context.CancelFunc
    done   chan struct{}
}

// В Dialog добавить:
type Dialog struct {
    // ... существующие поля
    
    // Управление фоновыми задачами
    tasks      map[string]*backgroundTask
    tasksMutex sync.Mutex
}

// Метод для запуска управляемой горутины
func (d *Dialog) runBackground(name string, fn func(ctx context.Context)) {
    d.tasksMutex.Lock()
    defer d.tasksMutex.Unlock()
    
    // Отменяем предыдущую задачу если есть
    if task, exists := d.tasks[name]; exists {
        task.cancel()
        <-task.done
    }
    
    ctx, cancel := context.WithCancel(d.ctx)
    task := &backgroundTask{
        cancel: cancel,
        done:   make(chan struct{}),
    }
    
    d.tasks[name] = task
    
    go func() {
        defer close(task.done)
        fn(ctx)
    }()
}

// В Close() добавить очистку задач:
func (d *Dialog) Close() error {
    // ... существующий код
    
    // Отменяем все фоновые задачи
    d.tasksMutex.Lock()
    for _, task := range d.tasks {
        task.cancel()
    }
    d.tasksMutex.Unlock()
    
    // Ждем завершения всех задач
    d.tasksMutex.Lock()
    for _, task := range d.tasks {
        <-task.done
    }
    d.tasks = nil
    d.tasksMutex.Unlock()
    
    return nil
}
```

### Фаза 3: Улучшение безопасности (1 день)

#### Задача 3.1: Усиление валидации
```go
// В security.go добавить:
const (
    MaxHeaderLength = 8192  // Максимальная длина заголовка
    MaxBodyLength   = 65536 // Максимальная длина тела
    MaxDialogsPerIP = 100   // Максимум диалогов с одного IP
)

// Расширить валидацию
func validateReferTo(referTo string) (*sip.Uri, map[string]string, error) {
    if len(referTo) > MaxHeaderLength {
        return nil, nil, fmt.Errorf("Refer-To header too long")
    }
    
    // Проверка на опасные символы
    if strings.ContainsAny(referTo, "\r\n\x00") {
        return nil, nil, fmt.Errorf("invalid characters in Refer-To")
    }
    
    // ... существующая логика парсинга с дополнительными проверками
}

// Добавить rate limiting в DialogManager
type DialogManager struct {
    // ... существующие поля
    
    // Rate limiting
    ipCounter map[string]int
    ipMutex   sync.Mutex
}

func (dm *DialogManager) checkRateLimit(remoteAddr string) error {
    dm.ipMutex.Lock()
    defer dm.ipMutex.Unlock()
    
    ip, _, _ := net.SplitHostPort(remoteAddr)
    count := dm.ipCounter[ip]
    
    if count >= MaxDialogsPerIP {
        return fmt.Errorf("too many dialogs from IP %s", ip)
    }
    
    dm.ipCounter[ip] = count + 1
    return nil
}
```

### Фаза 4: Улучшение обработки ошибок (1 день)

#### Задача 4.1: Добавление логирования
```go
// Создать logger.go
package dialog

import (
    "log"
    "os"
)

type Logger interface {
    Error(format string, args ...interface{})
    Warn(format string, args ...interface{})
    Info(format string, args ...interface{})
    Debug(format string, args ...interface{})
}

type defaultLogger struct {
    logger *log.Logger
}

func NewDefaultLogger() Logger {
    return &defaultLogger{
        logger: log.New(os.Stderr, "[dialog] ", log.LstdFlags|log.Lshortfile),
    }
}

func (l *defaultLogger) Error(format string, args ...interface{}) {
    l.logger.Printf("ERROR: "+format, args...)
}

// ... остальные методы

// В Dialog добавить:
type Dialog struct {
    // ... существующие поля
    logger Logger
}

// Использовать логирование вместо игнорирования ошибок:
if err := d.headerProcessor.ProcessRouteHeaders(req, d.routeSet); err != nil {
    d.logger.Warn("Failed to process route headers: %v", err)
}
```

#### Задача 4.2: Retry механизм
```go
// Создать retry.go
package dialog

import (
    "context"
    "time"
)

type RetryConfig struct {
    MaxAttempts     int
    InitialInterval time.Duration
    MaxInterval     time.Duration
    Multiplier      float64
}

var DefaultRetryConfig = RetryConfig{
    MaxAttempts:     3,
    InitialInterval: 100 * time.Millisecond,
    MaxInterval:     5 * time.Second,
    Multiplier:      2.0,
}

func RetryWithBackoff(ctx context.Context, config RetryConfig, fn func() error) error {
    var err error
    interval := config.InitialInterval
    
    for attempt := 0; attempt < config.MaxAttempts; attempt++ {
        err = fn()
        if err == nil {
            return nil
        }
        
        // Последняя попытка - не ждем
        if attempt == config.MaxAttempts-1 {
            break
        }
        
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(interval):
        }
        
        // Увеличиваем интервал
        interval = time.Duration(float64(interval) * config.Multiplier)
        if interval > config.MaxInterval {
            interval = config.MaxInterval
        }
    }
    
    return err
}
```

### Фаза 5: Оптимизация производительности (2 дня)

#### Задача 5.1: Добавление индексов в DialogManager
```go
type DialogManager struct {
    // ... существующие поля
    
    // Дополнительные индексы для быстрого поиска
    tagIndex      map[string]string  // fromTag:toTag -> dialogID
    remoteTagIndex map[string][]string // remoteTag -> []dialogID
    indexMutex    sync.RWMutex
}

// Оптимизированный поиск по ключу
func (dm *DialogManager) findDialogByKey(callID, localTag, remoteTag string) (IDialog, error) {
    dm.indexMutex.RLock()
    defer dm.indexMutex.RUnlock()
    
    // Быстрый поиск по составному ключу
    key := fmt.Sprintf("%s:%s", localTag, remoteTag)
    if dialogID, exists := dm.tagIndex[key]; exists {
        return dm.dialogs[dialogID], nil
    }
    
    return nil, ErrDialogNotFound
}

// Обновление индексов при добавлении диалога
func (dm *DialogManager) updateIndexes(dialog IDialog) {
    dm.indexMutex.Lock()
    defer dm.indexMutex.Unlock()
    
    dialogID := dialog.ID()
    localTag := dialog.LocalTag()
    remoteTag := dialog.RemoteTag()
    
    if localTag != "" && remoteTag != "" {
        key := fmt.Sprintf("%s:%s", localTag, remoteTag)
        dm.tagIndex[key] = dialogID
    }
    
    if remoteTag != "" {
        dm.remoteTagIndex[remoteTag] = append(dm.remoteTagIndex[remoteTag], dialogID)
    }
}
```

#### Задача 5.2: Пул горутин для обработки событий
```go
// Создать worker_pool.go
package dialog

import (
    "context"
    "sync"
)

type WorkerPool struct {
    workers   int
    taskQueue chan func()
    wg        sync.WaitGroup
    ctx       context.Context
    cancel    context.CancelFunc
}

func NewWorkerPool(workers int) *WorkerPool {
    ctx, cancel := context.WithCancel(context.Background())
    pool := &WorkerPool{
        workers:   workers,
        taskQueue: make(chan func(), workers*10),
        ctx:       ctx,
        cancel:    cancel,
    }
    
    pool.start()
    return pool
}

func (p *WorkerPool) start() {
    for i := 0; i < p.workers; i++ {
        p.wg.Add(1)
        go p.worker()
    }
}

func (p *WorkerPool) worker() {
    defer p.wg.Done()
    
    for {
        select {
        case <-p.ctx.Done():
            return
        case task, ok := <-p.taskQueue:
            if !ok {
                return
            }
            task()
        }
    }
}

func (p *WorkerPool) Submit(task func()) {
    select {
    case p.taskQueue <- task:
    case <-p.ctx.Done():
    }
}

func (p *WorkerPool) Stop() {
    p.cancel()
    close(p.taskQueue)
    p.wg.Wait()
}

// Использовать в DialogManager:
type DialogManager struct {
    // ... существующие поля
    workerPool *WorkerPool
}

func NewDialogManager(uasuac *UASUAC) *DialogManager {
    dm := &DialogManager{
        // ... инициализация
        workerPool: NewWorkerPool(10), // 10 воркеров
    }
    return dm
}
```

## Тестирование

### Unit тесты
1. Тесты на race conditions с использованием `go test -race`
2. Тесты на корректное закрытие ресурсов
3. Тесты на валидацию входных данных
4. Тесты производительности с бенчмарками

### Integration тесты
1. Тесты с реальным SIP сервером
2. Нагрузочные тесты
3. Тесты на устойчивость к некорректным данным

## Критерии успеха

1. **Отсутствие race conditions** - все тесты проходят с флагом `-race`
2. **Отсутствие утечек ресурсов** - профилирование показывает стабильное потребление памяти
3. **Безопасность** - устойчивость к fuzzing тестам
4. **Производительность** - обработка 10000 диалогов без деградации
5. **Надежность** - 99.9% успешных операций под нагрузкой

## График выполнения

- **Фаза 1**: 1-2 дня (критические race conditions)
- **Фаза 2**: 1-2 дня (утечки ресурсов)  
- **Фаза 3**: 1 день (безопасность)
- **Фаза 4**: 1 день (обработка ошибок)
- **Фаза 5**: 2 дня (производительность)
- **Тестирование**: 2 дня

**Итого**: 8-10 дней

## Следующие шаги

1. Начать с Фазы 1 - исправление критических race conditions
2. Создать тесты для воспроизведения проблем
3. Применить исправления
4. Валидировать с помощью race detector
5. Перейти к следующей фазе