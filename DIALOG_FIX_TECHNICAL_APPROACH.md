# Технический подход к исправлению проблем в пакете dialog

## Приоритизация исправлений

### Приоритет P0 (Критические - исправить немедленно)

#### 1. Race Condition в handleREFER (Влияние: Crash/Data corruption)
**Проблема**: Горутина запускается с доступом к полям диалога без синхронизации
**Решение**:
```go
// Копировать все необходимые данные под защитой мьютекса
func (d *Dialog) handleREFER(req *sip.Request, tx sip.ServerTransaction) error {
    // Создаем snapshot состояния под блокировкой
    snapshot := d.createSnapshot()
    
    // Работаем со snapshot в горутине
    go d.processREFERAsync(snapshot, event, subscription)
}
```

#### 2. Race Condition в updateDialogID (Влияние: Inconsistent state)
**Проблема**: Метод вызывается без защиты из разных горутин
**Решение**:
```go
// Всегда вызывать под защитой мьютекса
func (d *Dialog) updateDialogIDSafe() {
    d.mu.Lock()
    defer d.mu.Unlock()
    d.updateDialogIDUnsafe() // внутренняя версия
}
```

#### 3. Race Condition в applyDialogHeaders (Влияние: Incorrect CSeq)
**Проблема**: Инкремент localSeq без синхронизации
**Решение**:
```go
// Использовать atomic операции
type Dialog struct {
    localSeq atomic.Uint32
}

func (d *Dialog) applyDialogHeaders(req *sip.Request) {
    // Атомарный инкремент
    seq := d.localSeq.Add(1)
    req.ReplaceHeader(sip.NewHeader("CSeq", fmt.Sprintf("%d %s", seq, req.Method)))
}
```

#### 4. Утечка горутин (Влияние: Memory leak)
**Проблема**: Горутины не завершаются при закрытии диалога
**Решение**:
```go
// Использовать контекст для управления жизненным циклом
func (d *Dialog) runTask(name string, task func(context.Context)) {
    ctx, cancel := context.WithCancel(d.ctx)
    d.registerTask(name, cancel)
    
    go func() {
        defer d.unregisterTask(name)
        task(ctx)
    }()
}
```

#### 5. Отсутствие Close() в IDialog (Влияние: Resource leak)
**Проблема**: Невозможно корректно освободить ресурсы
**Решение**: Добавить метод Close() в интерфейс и реализацию

### Приоритет P1 (Важные - исправить в течение недели)

#### 1. Недостаточная валидация (Влияние: Security vulnerability)
**Решение**: Добавить строгую валидацию всех входных данных
```go
func validateAndSanitize(input string) (string, error) {
    // Проверка длины
    if len(input) > MaxHeaderLength {
        return "", ErrHeaderTooLong
    }
    // Проверка символов
    if !isValidSIPString(input) {
        return "", ErrInvalidCharacters
    }
    return input, nil
}
```

#### 2. Игнорирование ошибок (Влияние: Silent failures)
**Решение**: Логировать все ошибки
```go
if err := operation(); err != nil {
    d.logger.Error("operation failed: %v", err)
    // Определить стратегию: retry, fallback, или propagate
}
```

#### 3. Линейный поиск O(n) (Влияние: Poor performance)
**Решение**: Добавить индексы
```go
type DialogManager struct {
    // Основное хранилище
    dialogs map[string]IDialog
    
    // Индексы для быстрого поиска
    callIDIndex   map[string][]string // CallID -> []DialogID
    tagIndex      map[TagKey]string   // TagKey -> DialogID
}
```

## Технические решения по компонентам

### 1. Синхронизация и конкурентность

#### Паттерн: Copy-on-Read для горутин
```go
type DialogSnapshot struct {
    ID          string
    CallID      string
    LocalTag    string
    RemoteTag   string
    LocalURI    sip.Uri
    RemoteURI   sip.Uri
    // ... другие неизменяемые данные
}

func (d *Dialog) createSnapshot() DialogSnapshot {
    d.mu.RLock()
    defer d.mu.RUnlock()
    
    return DialogSnapshot{
        ID:        d.id,
        CallID:    d.callID.Value(),
        LocalTag:  d.localTag,
        RemoteTag: d.remoteTag,
        // ... копируем все необходимое
    }
}
```

#### Паттерн: Структурированная конкурентность
```go
type TaskManager struct {
    tasks map[string]context.CancelFunc
    mu    sync.Mutex
}

func (tm *TaskManager) Start(name string, fn func(context.Context)) {
    tm.mu.Lock()
    defer tm.mu.Unlock()
    
    // Отменяем предыдущую задачу
    if cancel, exists := tm.tasks[name]; exists {
        cancel()
    }
    
    ctx, cancel := context.WithCancel(context.Background())
    tm.tasks[name] = cancel
    
    go func() {
        defer tm.Remove(name)
        fn(ctx)
    }()
}

func (tm *TaskManager) StopAll() {
    tm.mu.Lock()
    defer tm.mu.Unlock()
    
    for _, cancel := range tm.tasks {
        cancel()
    }
    tm.tasks = make(map[string]context.CancelFunc)
}
```

### 2. Управление ресурсами

#### Паттерн: Resource Lifecycle
```go
type Resource interface {
    Init() error
    Close() error
    IsAlive() bool
}

type Dialog struct {
    // ... поля
    
    resources []Resource
    closed    atomic.Bool
}

func (d *Dialog) Close() error {
    if !d.closed.CompareAndSwap(false, true) {
        return nil // Уже закрыто
    }
    
    var errs []error
    
    // Закрываем в обратном порядке
    for i := len(d.resources) - 1; i >= 0; i-- {
        if err := d.resources[i].Close(); err != nil {
            errs = append(errs, err)
        }
    }
    
    if len(errs) > 0 {
        return fmt.Errorf("close errors: %v", errs)
    }
    
    return nil
}
```

### 3. Безопасность и валидация

#### Паттерн: Input Sanitization Pipeline
```go
type Validator func(string) error
type Sanitizer func(string) string

type ValidationPipeline struct {
    validators []Validator
    sanitizers []Sanitizer
}

func (vp *ValidationPipeline) Process(input string) (string, error) {
    // Валидация
    for _, v := range vp.validators {
        if err := v(input); err != nil {
            return "", err
        }
    }
    
    // Санитизация
    result := input
    for _, s := range vp.sanitizers {
        result = s(result)
    }
    
    return result, nil
}

// Использование
var headerPipeline = &ValidationPipeline{
    validators: []Validator{
        validateLength,
        validateCharacters,
        validateSIPSyntax,
    },
    sanitizers: []Sanitizer{
        trimWhitespace,
        normalizeCase,
    },
}
```

### 4. Производительность

#### Паттерн: Sharded Storage
```go
type ShardedDialogStore struct {
    shards []*DialogShard
    count  int
}

type DialogShard struct {
    mu      sync.RWMutex
    dialogs map[string]IDialog
    indexes map[string]map[string]string
}

func (s *ShardedDialogStore) getShard(key string) *DialogShard {
    h := fnv.New32()
    h.Write([]byte(key))
    return s.shards[h.Sum32()%uint32(s.count)]
}

func (s *ShardedDialogStore) Get(id string) (IDialog, error) {
    shard := s.getShard(id)
    shard.mu.RLock()
    defer shard.mu.RUnlock()
    
    dialog, exists := shard.dialogs[id]
    if !exists {
        return nil, ErrDialogNotFound
    }
    return dialog, nil
}
```

## Пошаговый план реализации

### День 1-2: Критические Race Conditions
1. **Час 1-2**: Написать тесты для воспроизведения race conditions
2. **Час 3-4**: Реализовать atomic операции для счетчиков
3. **Час 5-6**: Добавить snapshot паттерн для горутин
4. **Час 7-8**: Защитить все критические секции мьютексами
5. **День 2**: Тестирование с race detector

### День 3-4: Управление ресурсами
1. **Час 1-2**: Добавить Close() в интерфейс IDialog
2. **Час 3-4**: Реализовать TaskManager для горутин
3. **Час 5-6**: Добавить graceful shutdown
4. **Час 7-8**: Интеграционные тесты
5. **День 4**: Профилирование памяти

### День 5: Безопасность
1. **Час 1-2**: Реализовать ValidationPipeline
2. **Час 3-4**: Добавить rate limiting
3. **Час 5-6**: Fuzzing тесты
4. **Час 7-8**: Security review

### День 6: Обработка ошибок
1. **Час 1-2**: Добавить структурированное логирование
2. **Час 3-4**: Реализовать retry механизм
3. **Час 5-6**: Error wrapping и контекст
4. **Час 7-8**: Метрики и мониторинг

### День 7-8: Производительность
1. **День 7**: Реализовать индексы и sharding
2. **День 8**: Бенчмарки и оптимизация

## Метрики успеха

### Корректность
- ✅ `go test -race` проходит без ошибок
- ✅ Нет паник под нагрузкой
- ✅ Все ресурсы освобождаются корректно

### Производительность
- ✅ Поиск диалога < 1μs при 10K диалогов
- ✅ Создание диалога < 10μs
- ✅ Memory footprint < 1KB на диалог

### Надежность
- ✅ 99.99% uptime под нагрузкой
- ✅ Graceful degradation при перегрузке
- ✅ Автоматическое восстановление после сбоев

## Инструменты для валидации

### Race Detection
```bash
go test -race -count=10 ./pkg/dialog/...
```

### Memory Profiling
```bash
go test -memprofile=mem.prof -bench=.
go tool pprof mem.prof
```

### Fuzzing
```bash
go test -fuzz=FuzzParseReferTo -fuzztime=60s
```

### Load Testing
```bash
go test -bench=BenchmarkConcurrentDialogs -benchtime=60s
```