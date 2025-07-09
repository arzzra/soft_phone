# План действий по исправлению критических проблем в пакете dialog

## Обзор ситуации

На основе проведенного анализа выявлены критические проблемы в пакете `pkg/dialog`, которые могут привести к:
- Паникам и крашам приложения (race conditions)
- Утечкам памяти и горутин
- Уязвимостям безопасности
- Деградации производительности

## Приоритеты исправлений

### P0 - Критические (исправить в течение 1-2 дней)
1. **Race conditions** - могут привести к непредсказуемому поведению и крашам
2. **Утечки ресурсов** - приводят к деградации и eventual crash
3. **Null pointer dereference** - вызывают панику приложения

### P1 - Важные (исправить в течение недели)
1. **Безопасность** - уязвимости могут быть эксплуатированы
2. **Обработка ошибок** - silent failures усложняют отладку
3. **Производительность** - O(n) операции не масштабируются

## Детальный план исправлений

### Фаза 1: Race Conditions (День 1-2)

#### 1.1 handleREFER (dialog.go:687)
**Проблема**: Горутина обращается к полям диалога без синхронизации
```go
// Текущий код (ПРОБЛЕМА)
go func() {
    subscription.UpdateStatus(ReferStatusAccepted)
    if d.uasuac != nil && d.uasuac.client != nil { // Race!
        _ = subscription.SendNotify(d.ctx)
    }
}()
```

**Исправление**:
```go
// Копируем данные под защитой мьютекса
d.mu.RLock()
ctx := d.ctx
uasuac := d.uasuac
referHandler := d.referHandler
d.mu.RUnlock()

// Запускаем горутину с копиями
go func() {
    defer func() {
        if r := recover(); r != nil {
            // Логируем панику вместо краша
        }
    }()
    
    select {
    case <-ctx.Done():
        return
    default:
        // Обработка с проверками на nil
        if uasuac != nil && uasuac.client != nil {
            _ = subscription.SendNotify(ctx)
        }
    }
}()
```

#### 1.2 updateDialogID (dialog.go:141)
**Проблема**: Метод изменяет состояние без защиты
```go
// Текущий код (ПРОБЛЕМА)
func (d *Dialog) updateDialogID() {
    // Нет защиты мьютексом!
    if d.isServer {
        d.id = fmt.Sprintf(...)
    }
}
```

**Исправление**:
```go
func (d *Dialog) updateDialogID() {
    d.mu.Lock()
    defer d.mu.Unlock()
    
    oldID := d.id
    // Обновляем ID
    if d.isServer {
        d.id = fmt.Sprintf("%s;from-tag=%s;to-tag=%s", 
            d.callID.Value(), d.remoteTag, d.localTag)
    } else {
        d.id = fmt.Sprintf("%s;from-tag=%s;to-tag=%s", 
            d.callID.Value(), d.localTag, d.remoteTag)
    }
    
    // Уведомляем менеджер об изменении (синхронно)
    if oldID != d.id && d.uasuac != nil {
        d.uasuac.dialogManager.UpdateDialogID(oldID, d.id, d)
    }
}
```

#### 1.3 applyDialogHeaders (dialog.go:583)
**Проблема**: Инкремент localSeq без синхронизации
```go
// Текущий код (ПРОБЛЕМА)
d.localSeq++ // Race!
```

**Исправление**:
```go
// В структуре Dialog заменить:
type Dialog struct {
    // localSeq uint32 - старое
    localSeq atomic.Uint32 // новое
}

// В applyDialogHeaders:
func (d *Dialog) applyDialogHeaders(req *sip.Request) {
    // Безопасный инкремент
    seq := d.localSeq.Add(1)
    req.ReplaceHeader(sip.NewHeader("CSeq", 
        fmt.Sprintf("%d %s", seq, req.Method)))
}
```

### Фаза 2: Утечки ресурсов (День 3-4)

#### 2.1 Добавить Close() в интерфейс IDialog
```go
// В interfaces.go добавить:
type IDialog interface {
    // ... существующие методы
    
    // Close освобождает все ресурсы диалога
    Close() error
}
```

#### 2.2 Реализовать Close() в Dialog
```go
func (d *Dialog) Close() error {
    d.mu.Lock()
    defer d.mu.Unlock()
    
    // Проверяем, не закрыт ли уже
    if d.ctx.Err() != nil {
        return nil
    }
    
    // Отменяем контекст - это остановит все горутины
    d.cancel()
    
    // Переводим в терминальное состояние
    d.stateMachine.Event(context.Background(), "terminated")
    
    // Очищаем обработчики для GC
    d.stateChangeHandler = nil
    d.bodyHandler = nil
    d.requestHandler = nil
    d.referHandler = nil
    
    return nil
}
```

#### 2.3 Управление горутинами
```go
// Добавить в Dialog:
type Dialog struct {
    // ... существующие поля
    tasks sync.Map // map[string]context.CancelFunc
}

// Метод для запуска управляемых горутин
func (d *Dialog) runManagedTask(name string, fn func(context.Context)) {
    // Отменяем предыдущую задачу если есть
    if cancel, ok := d.tasks.Load(name); ok {
        cancel.(context.CancelFunc)()
    }
    
    ctx, cancel := context.WithCancel(d.ctx)
    d.tasks.Store(name, cancel)
    
    go func() {
        defer d.tasks.Delete(name)
        fn(ctx)
    }()
}
```

### Фаза 3: Null Pointer Checks (День 5)

#### 3.1 Добавить проверки во все критические места
```go
// Пример для handleREFER
func (d *Dialog) handleREFER(req *sip.Request, tx sip.ServerTransaction) error {
    // Проверки в начале метода
    if req == nil {
        return fmt.Errorf("request is nil")
    }
    
    if tx == nil {
        return fmt.Errorf("transaction is nil")
    }
    
    d.mu.RLock()
    if d.uasuac == nil {
        d.mu.RUnlock()
        res := sip.NewResponseFromRequest(req, 
            sip.StatusInternalServerError, 
            "Dialog not properly initialized", nil)
        return tx.Respond(res)
    }
    d.mu.RUnlock()
    
    // ... остальная логика
}
```

### Фаза 4: Безопасность (День 6)

#### 4.1 Валидация входных данных
```go
// В security.go добавить:
func validateReferToHeader(value string) error {
    // Проверка длины
    if len(value) > 8192 {
        return fmt.Errorf("header too long: %d bytes", len(value))
    }
    
    // Проверка на опасные символы
    if strings.ContainsAny(value, "\r\n\x00") {
        return fmt.Errorf("invalid characters in header")
    }
    
    return nil
}
```

#### 4.2 Rate Limiting
```go
// В DialogManager добавить:
type rateLimiter struct {
    mu    sync.Mutex
    count map[string]int
    reset time.Time
}

func (dm *DialogManager) checkRateLimit(ip string) error {
    dm.limiter.mu.Lock()
    defer dm.limiter.mu.Unlock()
    
    // Сброс счетчиков каждую минуту
    if time.Now().After(dm.limiter.reset) {
        dm.limiter.count = make(map[string]int)
        dm.limiter.reset = time.Now().Add(time.Minute)
    }
    
    if dm.limiter.count[ip] > 100 {
        return fmt.Errorf("rate limit exceeded for IP %s", ip)
    }
    
    dm.limiter.count[ip]++
    return nil
}
```

### Фаза 5: Обработка ошибок (День 7)

#### 5.1 Добавить логирование
```go
// Создать logger.go
type Logger interface {
    Error(format string, v ...interface{})
    Warn(format string, v ...interface{})
    Info(format string, v ...interface{})
    Debug(format string, v ...interface{})
}

// В Dialog добавить:
type Dialog struct {
    // ... существующие поля
    logger Logger
}

// Использовать вместо игнорирования:
if err := d.headerProcessor.ProcessRouteHeaders(req, d.routeSet); err != nil {
    d.logger.Warn("Failed to process route headers: %v", err)
    // Продолжаем выполнение, так как это не критично
}
```

### Фаза 6: Производительность (День 8-9)

#### 6.1 Добавить индексы
```go
type DialogManager struct {
    // Основное хранилище
    dialogs map[string]IDialog
    
    // Индексы
    callIDIndex map[string]string           // CallID -> DialogID
    tagIndex    map[string]string           // "localTag:remoteTag" -> DialogID
    
    mu sync.RWMutex
}

// Быстрый поиск по тегам
func (dm *DialogManager) GetDialogByTags(localTag, remoteTag string) (IDialog, error) {
    dm.mu.RLock()
    defer dm.mu.RUnlock()
    
    key := fmt.Sprintf("%s:%s", localTag, remoteTag)
    if dialogID, exists := dm.tagIndex[key]; exists {
        return dm.dialogs[dialogID], nil
    }
    
    return nil, ErrDialogNotFound
}
```

## Тестирование

### Тесты для каждой фазы

#### Фаза 1: Race condition тесты
```bash
# Запускать после каждого исправления
go test -race -count=100 ./pkg/dialog/...
```

#### Фаза 2: Тесты на утечки
```go
func TestDialogNoLeak(t *testing.T) {
    // Создаем много диалогов
    for i := 0; i < 1000; i++ {
        d := NewDialog(nil, false)
        go d.handleREFER(testReq, testTx)
        d.Close()
    }
    
    // Проверяем количество горутин
    runtime.GC()
    if runtime.NumGoroutine() > 10 {
        t.Errorf("Goroutine leak detected: %d", runtime.NumGoroutine())
    }
}
```

#### Фаза 3: Nil checks
```go
func TestNilSafety(t *testing.T) {
    d := &Dialog{}
    
    // Не должно паниковать
    err := d.handleREFER(nil, nil)
    assert.Error(t, err)
}
```

## Критерии успеха

1. ✅ Все race тесты проходят
2. ✅ Нет утечек памяти и горутин
3. ✅ Нет паник при некорректных данных
4. ✅ Производительность улучшена в 10x для поиска
5. ✅ 100% покрытие критического кода тестами

## Следующие шаги

1. **Немедленно**: Начать с Фазы 1 (race conditions)
2. **День 1**: Исправить handleREFER, updateDialogID, applyDialogHeaders
3. **День 2**: Провести race testing и исправить найденные проблемы
4. **День 3-4**: Реализовать управление ресурсами
5. **День 5-9**: Завершить остальные фазы
6. **День 10**: Финальное тестирование и документация

## Команда и ответственные

- **Race Conditions**: Senior разработчик с опытом concurrent programming
- **Resource Management**: Разработчик с опытом работы с горутинами
- **Security**: Security-focused разработчик
- **Testing**: QA инженер с опытом нагрузочного тестирования

## Риски

1. **Изменение интерфейса IDialog** - может сломать существующий код
   - Митигация: Сначала добавить Close() как отдельный интерфейс
   
2. **Производительность может временно ухудшиться** из-за дополнительных проверок
   - Митигация: Профилирование и оптимизация hot paths
   
3. **Сложность тестирования race conditions**
   - Митигация: Использовать stress тесты и fuzzing