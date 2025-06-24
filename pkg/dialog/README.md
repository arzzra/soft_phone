# Enhanced SIP Dialog Package - Три специализированных FSM

Пакет dialog реализует Enhanced SIP Dialog System с тремя специализированными конечными автоматами (FSM) для современных softphone приложений.

## 🚀 Архитектурная революция: Три FSM

### Новая архитектура

Enhanced SIP Dialog System теперь использует **три специализированных FSM** вместо одного единого:

1. **Dialog FSM** (`dialogFSM`) - управление состояниями SIP диалога (idle, calling, established, etc.)
2. **Transaction FSM** (`transactionFSM`) - управление SIP транзакциями (RFC 3261 transaction states)
3. **Timer FSM** (`timerFSM`) - управление RFC 3261 таймерами (Timer A, B, D, etc.)

### Преимущества разделения

- ✅ **Модульность**: Каждый FSM фокусируется на своей области ответственности
- ✅ **Масштабируемость**: Легче добавлять новые состояния и события
- ✅ **Читаемость**: Код становится более понятным и поддерживаемым
- ✅ **Тестирование**: Каждый FSM можно тестировать независимо
- ✅ **RFC соответствие**: Четкое разделение согласно RFC 3261

## 📁 Структура файлов

### Активные файлы (Enhanced Three FSM)

| Файл | Размер | Описание |
|------|--------|----------|
| `enhanced_dialog_three_fsm.go` | ~550 строк | **Основная реализация с тремя FSM** |
| `sip_stack_enhanced.go` | ~450 строк | Enhanced SIP Stack с sipgo интеграцией |
| `enhanced_handlers.go` | ~250 строк | Обработчики SIP сообщений |
| `enhanced_example.go` | ~310 строк | Примеры использования Enhanced системы |

### Документация

| Файл | Размер | Описание |
|------|--------|----------|
| `ENHANCED_SIP_DIALOG_THREE_FSM_DOCUMENTATION.md` | ~500 строк | **Полная документация трех FSM** |
| `README.md` | ~200 строк | Этот файл - краткий обзор |
| `FSM_TRANSACTIONS_SPECIFICATION.md` | ~485 строк | Спецификация FSM и транзакций |
| `SUMMARY.md` | ~245 строк | Итоговое резюме изменений |

### Удаленные файлы (старые реализации)

Удалены 8 файлов простых реализаций:
- `enhanced_dialog.go` (старый единый FSM)
- `sip_stack.go`, `sip_stack_simple.go`  
- `dialog.go`, `transaction.go`
- `sip_handlers.go`, `example_usage.go`
- `dialog_builder.go`, `sip_helpers.go`

## 🎯 Ключевые особенности

### 1. Три специализированных FSM

```go
type EnhancedSIPDialogThreeFSM struct {
    // Три специализированных FSM
    dialogFSM      *fsm.FSM              // Управление состояниями диалога
    transactionFSM *fsm.FSM              // Управление транзакциями
    timerFSM       *fsm.FSM              // Управление таймерами
    
    // Текущие состояния
    dialogState      EnhancedDialogState      // Состояние диалога
    transactionState EnhancedTransactionState // Состояние транзакции
    timerState       EnhancedTimerState       // Состояние таймеров
    
    // ... остальные поля
}
```

### 2. RFC 3261 Compliance

- **Dialog States**: idle, calling, proceeding, ringing, established, terminating, terminated, failed
- **Transaction States**: tx_idle, tx_trying, tx_proceeding, tx_completed, tx_confirmed, tx_terminated
- **Timer States**: timer_idle, timer_active, timer_expired, timer_stopped
- **RFC 3261 Timers**: Timer A (500ms), Timer B (32s), Timer D (32s), и др.

### 3. sipgo Integration

- Использование современной библиотеки `github.com/emiago/sipgo`
- DialogClientCache для UAC диалогов
- DialogServerCache для UAS диалогов  
- Актуальный API sipgo v0.33+

### 4. Thread Safety

- Все операции с диалогами thread-safe
- RWMutex для чтения/записи состояний
- Безопасная координация между FSM

### 5. Расширенная функциональность

- **RFC 3515 REFER**: Call transfer операции
- **RFC 3891 Replaces**: Call replacement функциональность
- **Custom Headers**: Поддержка кастомных SIP заголовков
- **Event System**: Callback система для мониторинга состояний

## 🚀 Quick Start

### Создание Enhanced SIP Stack

```go
package main

import (
    "github.com/arzzra/soft_phone/pkg/dialog"
)

func main() {
    // Конфигурация
    config := dialog.DefaultEnhancedSIPStackConfig()
    config.ListenAddr = "127.0.0.1:5060"
    config.Username = "alice"
    config.Domain = "example.com"
    config.EnableRefer = true
    config.EnableReplaces = true

    // Создание стека
    stack, err := dialog.NewEnhancedSIPStack(config)
    if err != nil {
        panic(err)
    }
    defer stack.Stop()

    // Запуск
    err = stack.Start()
    if err != nil {
        panic(err)
    }
}
```

### Исходящий звонок с тремя FSM

```go
// Создание диалога с тремя FSM
dialog, err := dialog.NewEnhancedThreeFSMOutgoingSIPDialog(
    stack, 
    "sip:bob@example.com", 
    "v=0\r\no=...", 
    map[string]string{"X-Call-Priority": "urgent"},
)
if err != nil {
    return err
}

// Отправка INVITE - координирует все три FSM
err = dialog.SendInvite()
if err != nil {
    return err
}

// Мониторинг состояний всех FSM
fmt.Printf("Dialog State: %s\n", dialog.GetDialogState())
fmt.Printf("Transaction State: %s\n", dialog.GetTransactionState())  
fmt.Printf("Timer State: %s\n", dialog.GetTimerState())
```

### Обработка событий

```go
stack.SetOnCallState(func(event *dialog.EnhancedCallStateEvent) {
    d := event.Dialog
    
    fmt.Printf("Dialog: %s -> %s\n", event.PrevState, event.State)
    fmt.Printf("Transaction: %s\n", d.GetTransactionState())
    fmt.Printf("Timer: %s\n", d.GetTimerState())
    
    switch event.State {
    case dialog.EStateEstablished:
        fmt.Printf("✅ Звонок установлен")
    case dialog.EStateTerminated:
        fmt.Printf("📞 Звонок завершен")
    case dialog.EStateFailed:
        fmt.Printf("❌ Звонок неуспешен")
    }
})
```

### Отмена исходящего звонка (CANCEL)

```go
// Создание и отправка INVITE
dialog, err := dialog.NewEnhancedThreeFSMOutgoingSIPDialog(stack, "sip:bob@example.com", sdp, nil)
err = dialog.SendInvite()

// Ждем немного (пока звонок в состоянии calling/proceeding/ringing)
time.Sleep(500 * time.Millisecond)

// Отменяем звонок
if dialog.GetDialogState() == dialog.EStateCalling || 
   dialog.GetDialogState() == dialog.EStateProceeding ||
   dialog.GetDialogState() == dialog.EStateRinging {
    
    err = dialog.SendCancel()
    if err != nil {
        log.Printf("Ошибка отправки CANCEL: %v", err)
    } else {
        fmt.Println("✅ Звонок отменен")
    }
}
```

### Перевод звонка (REFER)

```go
// Предполагаем, что диалог уже установлен (established)
if dialog.GetDialogState() == dialog.EStateEstablished {
    
    // Простой перевод звонка
    err = dialog.SendRefer("sip:charlie@example.com", "")
    if err != nil {
        log.Printf("Ошибка REFER: %v", err)
    } else {
        fmt.Println("📞 Звонок переведен")
    }
    
    // Перевод с заменой существующего звонка
    replaceCallID := "existing-call-123"
    err = dialog.SendRefer("sip:transfer-target@example.com", replaceCallID)
    if err != nil {
        log.Printf("Ошибка REFER с Replaces: %v", err)
    } else {
        fmt.Printf("🔄 Заменяем звонок %s\n", replaceCallID)
    }
}
```

### Завершение звонка

```go
// Graceful hangup - координирует Dialog и Transaction FSM
err = dialog.Hangup()

// Принудительное завершение
err = dialog.Terminate("User requested termination")
```

## 📊 API методы

### EnhancedSIPStack методы

```go
// Основные методы
func NewEnhancedSIPStack(config *EnhancedSIPStackConfig) (*EnhancedSIPStack, error)
func (s *EnhancedSIPStack) Start() error
func (s *EnhancedSIPStack) Stop()

// Создание звонков  
func (s *EnhancedSIPStack) MakeCall(target, sdp string, customHeaders map[string]string) (*EnhancedSIPDialog, error)
func (s *EnhancedSIPStack) MakeCallWithReplaces(target, sdp, replaceCallID, toTag, fromTag string, customHeaders map[string]string) (*EnhancedSIPDialog, error)

// Event callbacks
func (s *EnhancedSIPStack) SetOnCallState(callback func(*EnhancedCallStateEvent))
func (s *EnhancedSIPStack) SetOnIncomingCall(callback func(*EnhancedIncomingCallEvent))

// Custom headers
func (s *EnhancedSIPStack) AddCustomHeader(name, value string)
func (s *EnhancedSIPStack) RemoveCustomHeader(name string)

// Статистика
func (s *EnhancedSIPStack) GetStats() *EnhancedSIPStackStats
```

### EnhancedSIPDialogThreeFSM методы

```go
// Создание диалога
func NewEnhancedThreeFSMOutgoingSIPDialog(stack *EnhancedSIPStack, target, sdp string, customHeaders map[string]string) (*EnhancedSIPDialogThreeFSM, error)

// Управление звонками
func (d *EnhancedSIPDialogThreeFSM) SendInvite() error
func (d *EnhancedSIPDialogThreeFSM) SendCancel() error    // отмена исходящего звонка
func (d *EnhancedSIPDialogThreeFSM) Hangup() error  
func (d *EnhancedSIPDialogThreeFSM) Terminate(reason string) error

// REFER операции (RFC 3515)
func (d *EnhancedSIPDialogThreeFSM) SendRefer(referTo, replaceCallID string) error

// Получение состояний всех FSM
func (d *EnhancedSIPDialogThreeFSM) GetDialogState() EnhancedDialogState
func (d *EnhancedSIPDialogThreeFSM) GetTransactionState() EnhancedTransactionState
func (d *EnhancedSIPDialogThreeFSM) GetTimerState() EnhancedTimerState

// Получение данных
func (d *EnhancedSIPDialogThreeFSM) GetCallID() string
func (d *EnhancedSIPDialogThreeFSM) GetRemoteURI() string
func (d *EnhancedSIPDialogThreeFSM) GetLocalURI() string
func (d *EnhancedSIPDialogThreeFSM) GetSDP() string
func (d *EnhancedSIPDialogThreeFSM) GetCustomHeaders() map[string]string
func (d *EnhancedSIPDialogThreeFSM) GetDirection() string

// Методы совместимости со старым API
func (d *EnhancedSIPDialogThreeFSM) GetState() EnhancedDialogState // алиас для GetDialogState()
```

## 🔄 Координация FSM

### Взаимодействие FSM

1. **Dialog → Transaction**: Dialog FSM инициирует транзакции
2. **Transaction → Timer**: Transaction FSM запускает таймеры  
3. **Timer → Transaction**: Timer FSM уведомляет о тайм-аутах
4. **Transaction → Dialog**: Transaction FSM обновляет Dialog FSM

### Пример: Исходящий INVITE

```
1. Dialog FSM:      idle → calling (send_invite)
2. Transaction FSM: idle → trying (tx_start) 
3. Timer FSM:       idle → active (timer_start) - запуск Timer A и B

При получении 2xx:
1. Timer FSM:       active → stopped (timer_stop)
2. Transaction FSM: trying → completed (tx_receive_2xx)
3. Dialog FSM:      calling → established (receive_2xx)
```

## 🧪 Примеры и тестирование

### Запуск примеров

```bash
# Базовый пример Enhanced SIP Stack
go run cmd/test_sip/main.go -mode=example

# Все примеры Enhanced системы
go run pkg/dialog/enhanced_example.go
```

### Компиляция проекта

```bash
# Компиляция всего проекта
go build ./...

# Компиляция specific пакета
go build ./pkg/dialog
```

## 🔧 Конфигурация

### EnhancedSIPStackConfig

```go
type EnhancedSIPStackConfig struct {
    ListenAddr      string            // Адрес для прослушивания
    Username        string            // SIP username
    Domain          string            // SIP domain
    EnableRefer     bool              // Включить RFC 3515 REFER
    EnableReplaces  bool              // Включить RFC 3891 Replaces  
    CustomHeaders   map[string]string // Глобальные кастомные заголовки
    UserAgent       string            // User-Agent заголовок
}
```

### Настройки по умолчанию

```go
func DefaultEnhancedSIPStackConfig() *EnhancedSIPStackConfig {
    return &EnhancedSIPStackConfig{
        ListenAddr:      "127.0.0.1:5060",
        Username:        "enhanced_user",
        Domain:          "localhost",
        EnableRefer:     true,
        EnableReplaces:  true,
        CustomHeaders:   make(map[string]string),
        UserAgent:       "Enhanced-SIP-Stack/1.0",
    }
}
```

## 📈 Производительность и статистика

### EnhancedSIPStackStats

```go
type EnhancedSIPStackStats struct {
    TotalInvites    int64 // Общее количество INVITE
    TotalByes       int64 // Общее количество BYE
    TotalRefers     int64 // Общее количество REFER  
    ActiveDialogs   int64 // Активные диалоги
    FailedCalls     int64 // Неуспешные звонки
    Uptime          time.Duration // Время работы
}
```

## 🔗 Совместимость

### Обратная совместимость

Новая архитектура полностью совместима со старым API благодаря алиасам:

```go
// Алиасы типов для совместимости
type EnhancedSIPDialog = EnhancedSIPDialogThreeFSM
type EnhancedDialogDirection = string

// Совместимые конструкторы
func NewEnhancedOutgoingSIPDialog(...) (*EnhancedSIPDialog, error)
func NewEnhancedIncomingSIPDialog(...) (*EnhancedSIPDialog, error)
```

Существующий код продолжает работать без изменений!

## 📚 Дополнительная документация

- **[ENHANCED_SIP_DIALOG_THREE_FSM_DOCUMENTATION.md](ENHANCED_SIP_DIALOG_THREE_FSM_DOCUMENTATION.md)** - Полная техническая документация трех FSM
- **[FSM_TRANSACTIONS_SPECIFICATION.md](FSM_TRANSACTIONS_SPECIFICATION.md)** - Детальная спецификация FSM и транзакций  
- **[SUMMARY.md](SUMMARY.md)** - Итоговое резюме всех изменений

## 🎉 Заключение

Enhanced SIP Dialog System с тремя специализированными FSM представляет современную, модульную и RFC-совместимую реализацию для production softphone приложений. 

**Ключевые достижения:**
- ✅ Модульная архитектура с тремя FSM
- ✅ Полное RFC 3261/3515/3891 соответствие
- ✅ sipgo интеграция для современного SIP стека
- ✅ Thread-safe операции и координация FSM
- ✅ Обратная совместимость API
- ✅ Comprehensive документация и примеры

Система готова для использования в серьезных производственных softphone приложениях! 🚀 