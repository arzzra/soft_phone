# План исправления критических проблем пакета RTP

## Общая информация

Данный документ содержит детальный план исправления критических проблем, выявленных в ходе комплексного обзора пакета RTP. План предназначен для Agent D и содержит четкие инструкции, критерии приемки и требования к тестированию.

## Приоритеты и временные рамки

- **P0 (Критические)**: 1-2 дня - ДОЛЖНЫ быть исправлены немедленно
- **P1 (Важные)**: 3-5 дней - исправляются после P0
- **P2 (Улучшения)**: 1 неделя - исправляются после P1

## P0: Критические исправления (Начать немедленно)

### 1. Исправление игнорируемых ошибок

**Файлы для проверки и исправления:**
- `session.go` (строки 262, 683, 690)
- `rtp_session.go`
- `rtcp_session.go`
- `transport_*.go`
- Все тестовые файлы

**Что нужно сделать:**

1. Найти ВСЕ места, где игнорируются ошибки:
   ```bash
   # Использовать grep для поиска игнорируемых ошибок
   grep -n "_ =" pkg/rtp/*.go
   grep -n "^[[:space:]]*[a-zA-Z_][a-zA-Z0-9_]*\.[a-zA-Z_][a-zA-Z0-9_]*(" pkg/rtp/*.go | grep -v "if err"
   ```

2. Для каждого найденного места:
   - Добавить проверку ошибки
   - Логировать ошибку если она не критична
   - Возвращать ошибку если она критична
   - Добавить cleanup при необходимости

**Пример исправления:**
```go
// БЫЛО:
s.rtpSession.Stop()

// ДОЛЖНО БЫТЬ:
if err := s.rtpSession.Stop(); err != nil {
    s.logger.Error("failed to stop RTP session", "error", err)
    return fmt.Errorf("failed to stop RTP session: %w", err)
}
```

**Критерии приемки:**
- [ ] Все ошибки обрабатываются корректно
- [ ] Добавлено логирование для некритичных ошибок
- [ ] Критичные ошибки прокидываются наверх
- [ ] Код проходит `golangci-lint run`

### 2. Исправление копирования структур с мьютексами

**Файл:** `metrics_collector.go` строка 245

**Проблема:**
```go
result := *mc.globalStats  // КРИТИЧЕСКАЯ ОШИБКА - копирует sync.RWMutex!
```

**Решение:**
1. Создать метод для безопасного копирования данных:
```go
func (mc *MetricsCollector) GetGlobalStats() SessionMetrics {
    mc.mu.RLock()
    defer mc.mu.RUnlock()
    
    // Копируем только данные, не мьютексы
    return SessionMetrics{
        PacketsSent:     mc.globalStats.PacketsSent,
        PacketsReceived: mc.globalStats.PacketsReceived,
        BytesSent:       mc.globalStats.BytesSent,
        BytesReceived:   mc.globalStats.BytesReceived,
        PacketsLost:     mc.globalStats.PacketsLost,
        // ... копировать все поля явно
    }
}
```

2. Проверить ВСЕ структуры с мьютексами на предмет копирования:
```bash
# Найти все структуры с мьютексами
grep -n "sync\.\(Mutex\|RWMutex\)" pkg/rtp/*.go
```

**Критерии приемки:**
- [ ] Нет копирования структур с мьютексами
- [ ] Все методы возвращают копии данных, а не структур
- [ ] Race detector не находит проблем: `go test -race ./pkg/rtp/...`

### 3. Устранение утечек ресурсов

**Файлы для проверки:**
- `transport_dtls.go`
- `session.go`
- `rtp_session.go`
- `rtcp_session.go`

**Что нужно сделать:**

1. Добавить graceful shutdown с контекстом:
```go
func (s *Session) Stop(ctx context.Context) error {
    // Установить флаг остановки
    s.stopOnce.Do(func() {
        close(s.stopCh)
    })
    
    // Создать WaitGroup для всех горутин
    var wg sync.WaitGroup
    
    // Остановить все подсистемы параллельно
    wg.Add(3)
    
    errCh := make(chan error, 3)
    
    go func() {
        defer wg.Done()
        if err := s.rtpSession.Stop(ctx); err != nil {
            errCh <- fmt.Errorf("RTP session stop: %w", err)
        }
    }()
    
    go func() {
        defer wg.Done()
        if err := s.rtcpSession.Stop(ctx); err != nil {
            errCh <- fmt.Errorf("RTCP session stop: %w", err)
        }
    }()
    
    go func() {
        defer wg.Done()
        if err := s.transport.Close(); err != nil {
            errCh <- fmt.Errorf("transport close: %w", err)
        }
    }()
    
    // Ждать завершения с таймаутом
    done := make(chan struct{})
    go func() {
        wg.Wait()
        close(done)
        close(errCh)
    }()
    
    select {
    case <-done:
        // Собрать все ошибки
        var errs []error
        for err := range errCh {
            errs = append(errs, err)
        }
        if len(errs) > 0 {
            return fmt.Errorf("stop errors: %v", errs)
        }
        return nil
    case <-ctx.Done():
        return fmt.Errorf("stop timeout: %w", ctx.Err())
    }
}
```

2. Добавить defer cleanup во все места инициализации:
```go
func (t *DTLSTransport) Start() error {
    // Список функций для cleanup
    var cleanupFuncs []func()
    
    // Флаг успешной инициализации
    var initialized bool
    
    // Cleanup при ошибке
    defer func() {
        if !initialized {
            for _, fn := range cleanupFuncs {
                fn()
            }
        }
    }()
    
    // Создаем listener
    listener, err := dtls.Listen("udp", t.addr, t.config)
    if err != nil {
        return fmt.Errorf("dtls listen: %w", err)
    }
    cleanupFuncs = append(cleanupFuncs, func() { listener.Close() })
    
    // ... остальная инициализация
    
    initialized = true
    return nil
}
```

**Критерии приемки:**
- [ ] Все ресурсы корректно освобождаются
- [ ] Graceful shutdown работает с таймаутами
- [ ] Нет зависших горутин после остановки
- [ ] Нет утечек файловых дескрипторов

### 4. Исправление race conditions в обработчиках событий

**Проблема:** Обработчики событий могут изменяться во время выполнения

**Решение:**
1. Использовать sync.Map или копирование под блокировкой:
```go
type EventHandlers struct {
    mu       sync.RWMutex
    handlers map[EventType][]EventHandler
}

func (eh *EventHandlers) Emit(event Event) {
    eh.mu.RLock()
    // Копируем слайс обработчиков
    handlers := make([]EventHandler, len(eh.handlers[event.Type]))
    copy(handlers, eh.handlers[event.Type])
    eh.mu.RUnlock()
    
    // Вызываем обработчики вне блокировки
    for _, handler := range handlers {
        handler(event)
    }
}

func (eh *EventHandlers) Register(eventType EventType, handler EventHandler) {
    eh.mu.Lock()
    defer eh.mu.Unlock()
    
    eh.handlers[eventType] = append(eh.handlers[eventType], handler)
}
```

**Критерии приемки:**
- [ ] Race detector не находит проблем
- [ ] Обработчики можно безопасно добавлять/удалять во время работы
- [ ] Нет deadlock'ов

## Исправления безопасности (Высокий приоритет)

### 5. Защита от множественных DTLS соединений

**Файл:** `transport_dtls.go` строка 210

**Решение:**
```go
func (t *DTLSTransport) acceptDTLSConnection() {
    for {
        conn, err := t.listener.Accept()
        if err != nil {
            if !t.isStopped() {
                t.logger.Error("DTLS accept error", "error", err)
            }
            return
        }
        
        // Проверяем, есть ли уже соединение от этого адреса
        addr := conn.RemoteAddr().String()
        
        t.connMu.Lock()
        if existingConn, exists := t.connections[addr]; exists {
            t.connMu.Unlock()
            
            // Закрываем новое соединение
            conn.Close()
            t.logger.Warn("rejected duplicate DTLS connection", "addr", addr)
            continue
        }
        
        // Добавляем новое соединение
        t.connections[addr] = conn
        t.connMu.Unlock()
        
        // Обрабатываем в отдельной горутине
        go t.handleConnection(conn)
    }
}
```

**Критерии приемки:**
- [ ] Только одно соединение на IP адрес
- [ ] Старые соединения корректно закрываются
- [ ] Есть логирование попыток множественных соединений

### 6. Валидация входных данных в SendAudio

**Что проверять:**
```go
func (s *RTPSession) SendAudio(data []byte, timestamp uint32) error {
    // Валидация размера
    if len(data) == 0 {
        return fmt.Errorf("audio data is empty")
    }
    
    if len(data) > MaxRTPPacketSize {
        return fmt.Errorf("audio data too large: %d > %d", len(data), MaxRTPPacketSize)
    }
    
    // Валидация timestamp
    if timestamp < s.lastTimestamp {
        return fmt.Errorf("timestamp regression: %d < %d", timestamp, s.lastTimestamp)
    }
    
    // Проверка состояния сессии
    if !s.isActive() {
        return fmt.Errorf("session is not active")
    }
    
    // ... остальная логика
}
```

**Критерии приемки:**
- [ ] Все входные параметры валидируются
- [ ] Размеры проверяются против констант
- [ ] Ошибки возвращаются с понятными сообщениями

### 7. Реализация Rate Limiting

**Создать новый файл:** `rate_limiter.go`

```go
package rtp

import (
    "sync"
    "time"
    "golang.org/x/time/rate"
)

type RateLimiter struct {
    mu       sync.RWMutex
    limiters map[string]*rate.Limiter
    
    // Конфигурация
    ratePerSecond int
    burst         int
    cleanupInterval time.Duration
}

func NewRateLimiter(rps, burst int) *RateLimiter {
    rl := &RateLimiter{
        limiters:      make(map[string]*rate.Limiter),
        ratePerSecond: rps,
        burst:         burst,
        cleanupInterval: 5 * time.Minute,
    }
    
    go rl.cleanup()
    return rl
}

func (rl *RateLimiter) Allow(key string) bool {
    rl.mu.Lock()
    limiter, exists := rl.limiters[key]
    if !exists {
        limiter = rate.NewLimiter(rate.Limit(rl.ratePerSecond), rl.burst)
        rl.limiters[key] = limiter
    }
    rl.mu.Unlock()
    
    return limiter.Allow()
}

func (rl *RateLimiter) cleanup() {
    ticker := time.NewTicker(rl.cleanupInterval)
    defer ticker.Stop()
    
    for range ticker.C {
        rl.mu.Lock()
        // Удаляем неактивные лимитеры
        for key, limiter := range rl.limiters {
            if limiter.Tokens() >= float64(rl.burst) {
                delete(rl.limiters, key)
            }
        }
        rl.mu.Unlock()
    }
}
```

**Интеграция в транспорт:**
```go
func (t *UDPTransport) handleIncomingPackets() {
    // Создаем rate limiter: 1000 пакетов в секунду, burst 100
    limiter := NewRateLimiter(1000, 100)
    
    for {
        n, addr, err := t.conn.ReadFromUDP(t.buffer)
        if err != nil {
            // ... обработка ошибки
        }
        
        // Rate limiting по IP
        if !limiter.Allow(addr.IP.String()) {
            t.metrics.IncrementDroppedPackets()
            continue
        }
        
        // ... обработка пакета
    }
}
```

**Критерии приемки:**
- [ ] Rate limiting работает per-IP
- [ ] Конфигурируемые лимиты
- [ ] Метрики для отброшенных пакетов
- [ ] Автоматическая очистка неактивных лимитеров

## Оптимизация производительности

### 8. Buffer Pool для уменьшения аллокаций

**Создать файл:** `buffer_pool.go`

```go
package rtp

import (
    "sync"
)

const (
    MaxRTPPacketSize = 1500
    MaxRTCPPacketSize = 1024
)

var (
    rtpBufferPool = sync.Pool{
        New: func() interface{} {
            buf := make([]byte, MaxRTPPacketSize)
            return &buf
        },
    }
    
    rtcpBufferPool = sync.Pool{
        New: func() interface{} {
            buf := make([]byte, MaxRTCPPacketSize)
            return &buf
        },
    }
)

func GetRTPBuffer() *[]byte {
    return rtpBufferPool.Get().(*[]byte)
}

func PutRTPBuffer(buf *[]byte) {
    if buf != nil && len(*buf) == MaxRTPPacketSize {
        rtpBufferPool.Put(buf)
    }
}

func GetRTCPBuffer() *[]byte {
    return rtcpBufferPool.Get().(*[]byte)
}

func PutRTCPBuffer(buf *[]byte) {
    if buf != nil && len(*buf) == MaxRTCPPacketSize {
        rtcpBufferPool.Put(buf)
    }
}
```

**Использование в транспорте:**
```go
func (t *UDPTransport) handleIncomingPackets() {
    for {
        // Получаем буфер из пула
        bufPtr := GetRTPBuffer()
        buffer := *bufPtr
        
        n, addr, err := t.conn.ReadFromUDP(buffer)
        if err != nil {
            PutRTPBuffer(bufPtr) // Возвращаем в пул
            continue
        }
        
        // Копируем только нужную часть
        packet := make([]byte, n)
        copy(packet, buffer[:n])
        
        // Возвращаем буфер в пул
        PutRTPBuffer(bufPtr)
        
        // Обрабатываем пакет
        t.handlePacket(packet, addr)
    }
}
```

**Критерии приемки:**
- [ ] Уменьшение аллокаций минимум на 50%
- [ ] Benchmarks показывают улучшение производительности
- [ ] Нет утечек памяти

## Улучшения качества кода

### 9. Замена магических чисел

**Создать файл:** `constants.go`

```go
package rtp

const (
    // Размеры пакетов
    DefaultAveragePacketSize = 200 // bytes, based on typical audio codec
    MaxRTPPacketSize         = 1500 // MTU size
    MaxRTCPPacketSize        = 1024 // RFC recommendation
    
    // Таймауты
    DefaultSessionTimeout    = 30 * time.Second
    DefaultKeepalivePeriod   = 10 * time.Second
    DTLSHandshakeTimeout     = 5 * time.Second
    
    // Rate limiting
    DefaultRateLimit         = 1000  // packets per second
    DefaultBurstSize         = 100   // packets
    
    // Platform specific
    SO_TRAFFIC_CLASS_DARWIN  = 0x1001 // macOS specific socket option
    
    // RTCP intervals (RFC 3550)
    MinRTCPInterval          = 5 * time.Second
    MaxRTCPBandwidthFraction = 0.05 // 5% of session bandwidth
)
```

**Критерии приемки:**
- [ ] Все магические числа вынесены в константы
- [ ] Константы документированы
- [ ] Код использует константы вместо чисел

### 10. Удаление неиспользуемого кода

**Что удалить:**
1. Файл `transport_extended_test.go.broken`
2. Неиспользуемые поля в структурах
3. Неиспользуемые helper функции

```bash
# Найти неиспользуемые экспорты
golangci-lint run --enable=unused

# Удалить сломанный файл
rm pkg/rtp/transport_extended_test.go.broken
```

**Критерии приемки:**
- [ ] Нет неиспользуемого кода
- [ ] Нет сломанных тестов
- [ ] golangci-lint не находит unused проблем

## Требования к тестированию

### Для каждого исправления:

1. **Unit тесты:**
   - Покрытие минимум 80%
   - Тесты на граничные случаи
   - Тесты на ошибки

2. **Integration тесты:**
   - Тест полного цикла работы сессии
   - Тест graceful shutdown
   - Тест обработки ошибок

3. **Race тесты:**
   ```bash
   go test -race -count=10 ./pkg/rtp/...
   ```

4. **Benchmark тесты:**
   ```bash
   go test -bench=. -benchmem ./pkg/rtp/...
   ```

5. **Stress тесты:**
   - Запустить существующие stress тесты
   - Убедиться в отсутствии утечек памяти

## Финальная проверка качества

После завершения ВСЕХ исправлений:

1. **Запустить линтер:**
   ```bash
   make lint
   # или
   golangci-lint run
   ```

2. **Запустить все тесты:**
   ```bash
   go test -v -race -cover ./pkg/rtp/...
   ```

3. **Проверить производительность:**
   ```bash
   go test -bench=. -benchmem ./pkg/rtp/... > bench_after.txt
   # Сравнить с bench_before.txt
   ```

4. **Проверить на утечки:**
   ```bash
   go test -memprofile=mem.prof ./pkg/rtp/...
   go tool pprof mem.prof
   ```

## Порядок выполнения

1. **День 1:**
   - Исправить ВСЕ игнорируемые ошибки (P0-1)
   - Исправить копирование мьютексов (P0-2)
   - Запустить тесты, убедиться что ничего не сломано

2. **День 2:**
   - Реализовать graceful shutdown (P0-3)
   - Исправить race conditions (P0-4)
   - Исправить DTLS уязвимость (SEC-1)

3. **День 3:**
   - Добавить валидацию данных (SEC-2)
   - Реализовать rate limiting (SEC-3)
   - Начать оптимизацию памяти

4. **День 4-5:**
   - Завершить оптимизации
   - Улучшения качества кода
   - Финальное тестирование

## Важные замечания для Agent D

1. **НЕ меняйте публичный API** без крайней необходимости
2. **Добавляйте тесты** для каждого исправления
3. **Документируйте** все изменения в комментариях
4. **Используйте атомарные коммиты** - один коммит на одно исправление
5. **Запускайте линтер** после каждого изменения

## Критерии успеха

- [ ] Все P0 проблемы исправлены
- [ ] Все тесты проходят
- [ ] Race detector не находит проблем
- [ ] Линтер не выдает ошибок
- [ ] Производительность не ухудшилась
- [ ] Нет новых уязвимостей безопасности

После выполнения всех задач пакет RTP будет готов к production использованию!