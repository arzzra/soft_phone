# Последовательность исправления критических проблем RTP

## Шаг 1: Подготовка к работе

```bash
# 1. Создать новую ветку для исправлений
git checkout -b fix/rtp-critical-issues

# 2. Сделать baseline для производительности
go test -bench=. -benchmem ./pkg/rtp/... > benchmark_before.txt

# 3. Запустить тесты и сохранить результат
go test -v ./pkg/rtp/... > tests_before.txt

# 4. Проверить текущие проблемы линтера
golangci-lint run ./pkg/rtp/... > lint_before.txt
```

## Шаг 2: Исправление игнорируемых ошибок (КРИТИЧНО!)

### 2.1 Найти все проблемные места

```bash
# Поиск игнорируемых ошибок
cd pkg/rtp
grep -n "_ =" *.go
grep -n "^\s*[a-zA-Z_][a-zA-Z0-9_]*\.[a-zA-Z_][a-zA-Z0-9_]*(" *.go | grep -v "if err" | grep -v "return"
```

### 2.2 Исправить session.go:262

```go
// Найти строку 262 в session.go
// БЫЛО:
tx.Rollback()

// ИСПРАВИТЬ НА:
if rbErr := tx.Rollback(); rbErr != nil {
    s.logger.Error("failed to rollback transaction", 
        "error", rbErr, 
        "original_error", err)
}
```

### 2.3 Исправить session.go:683, 690

```go
// Найти binary.Read без проверки ошибки
// БЫЛО:
binary.Read(reader, binary.BigEndian, &value)

// ИСПРАВИТЬ НА:
if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
    return fmt.Errorf("failed to read value: %w", err)
}
```

### 2.4 Проверить результат

```bash
# После исправлений запустить тесты для session.go
go test -v ./pkg/rtp -run TestSession
```

## Шаг 3: Исправление копирования мьютексов (КРИТИЧНО!)

### 3.1 Найти проблему в metrics_collector.go:245

```go
// НАЙТИ:
result := *mc.globalStats  // Строка 245

// ЗАМЕНИТЬ НА:
result := mc.getGlobalStatsCopy()

// ДОБАВИТЬ НОВЫЙ МЕТОД:
func (mc *MetricsCollector) getGlobalStatsCopy() SessionMetrics {
    mc.mu.RLock()
    defer mc.mu.RUnlock()
    
    // Создаем новую структуру и копируем только данные
    return SessionMetrics{
        PacketsSent:     mc.globalStats.PacketsSent,
        PacketsReceived: mc.globalStats.PacketsReceived,
        BytesSent:       mc.globalStats.BytesSent,
        BytesReceived:   mc.globalStats.BytesReceived,
        PacketsLost:     mc.globalStats.PacketsLost,
        Jitter:          mc.globalStats.Jitter,
        RTT:             mc.globalStats.RTT,
        // Копировать ВСЕ поля кроме мьютексов
    }
}
```

### 3.2 Проверить на race conditions

```bash
go test -race ./pkg/rtp -run TestMetrics
```

## Шаг 4: Исправление DTLS уязвимости

### 4.1 Найти transport_dtls.go:210

```go
// НАЙТИ метод acceptDTLSConnection около строки 210

// ДОБАВИТЬ В НАЧАЛО СТРУКТУРЫ DTLSTransport:
type DTLSTransport struct {
    // ... существующие поля
    
    connMu      sync.RWMutex
    connections map[string]net.Conn  // Добавить это поле
}

// ИСПРАВИТЬ acceptDTLSConnection:
func (t *DTLSTransport) acceptDTLSConnection() {
    for {
        conn, err := t.listener.Accept()
        if err != nil {
            // ... существующая обработка
        }
        
        addr := conn.RemoteAddr().String()
        
        // Проверяем существующее соединение
        t.connMu.Lock()
        if oldConn, exists := t.connections[addr]; exists {
            // Закрываем старое соединение
            oldConn.Close()
            t.logger.Warn("closing old DTLS connection", "addr", addr)
        }
        t.connections[addr] = conn
        t.connMu.Unlock()
        
        go t.handleConnection(conn)
    }
}
```

## Шаг 5: Добавить валидацию в SendAudio

### 5.1 Найти метод SendAudio

```go
// ДОБАВИТЬ В НАЧАЛО МЕТОДА:
func (s *RTPSession) SendAudio(data []byte, timestamp uint32) error {
    // Валидация входных данных
    if len(data) == 0 {
        return fmt.Errorf("SendAudio: data cannot be empty")
    }
    
    const MaxAudioPacketSize = 1400 // MTU - headers
    if len(data) > MaxAudioPacketSize {
        return fmt.Errorf("SendAudio: data too large (%d bytes), max %d", 
            len(data), MaxAudioPacketSize)
    }
    
    // Проверка активности сессии
    s.mu.RLock()
    active := s.active
    s.mu.RUnlock()
    
    if !active {
        return fmt.Errorf("SendAudio: session is not active")
    }
    
    // ... остальной код метода
}
```

## Шаг 6: Быстрая проверка исправлений

```bash
# 1. Запустить тесты с race detector
go test -race -count=3 ./pkg/rtp/...

# 2. Запустить линтер
golangci-lint run ./pkg/rtp/...

# 3. Если все хорошо, закоммитить
git add -A
git commit -m "fix(rtp): Исправлены критические проблемы безопасности и стабильности

- Добавлена обработка всех игнорируемых ошибок в session.go
- Исправлено копирование структур с мьютексами в metrics_collector.go
- Устранена уязвимость множественных DTLS соединений
- Добавлена валидация входных данных в SendAudio()

Эти исправления устраняют критические проблемы, выявленные в code review."
```

## Шаг 7: Экстренные меры если что-то сломалось

Если после исправлений тесты падают:

```bash
# 1. Посмотреть что именно сломалось
go test -v ./pkg/rtp/... | grep FAIL

# 2. Откатить конкретное изменение
git diff HEAD~1 path/to/file.go

# 3. Или откатить все изменения
git checkout -- .

# 4. Начать исправления по одному с проверкой после каждого
```

## Чек-лист критических исправлений

- [ ] session.go:262 - обработка ошибки rollback
- [ ] session.go:683 - обработка ошибки binary.Read
- [ ] session.go:690 - обработка ошибки binary.Read  
- [ ] metrics_collector.go:245 - исправлено копирование мьютекса
- [ ] transport_dtls.go:210 - защита от множественных соединений
- [ ] SendAudio() - добавлена валидация размера данных
- [ ] Все тесты проходят с -race флагом
- [ ] golangci-lint не показывает новых ошибок

## Важно!

1. **Делайте одно исправление за раз** и проверяйте тесты
2. **Не меняйте публичные интерфейсы** без необходимости
3. **Сохраняйте обратную совместимость**
4. **Документируйте** каждое изменение в комментариях

После выполнения этих шагов, пакет станет значительно стабильнее и безопаснее!