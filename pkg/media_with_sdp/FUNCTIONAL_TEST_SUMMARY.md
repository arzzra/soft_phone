# Функциональный тест media_with_sdp

## 🎯 Цель теста

Демонстрация полного цикла SDP переговоров и обмена данными между двумя медиа сессиями на localhost, включая:

1. **SDP переговоры**: Offer/Answer обмен
2. **Управление портами**: Автоматическое выделение RTP/RTCP портов
3. **Обмен аудио данными**: Отправка/получение аудио пакетов
4. **DTMF сигналы**: Передача тональных сигналов
5. **Менеджер сессий**: Централизованное управление

## 📁 Файлы теста

### `examples/media_with_sdp_functional_test/main.go`
Полный функциональный тест с расширенной функциональностью:
- Детальное логирование всех этапов
- Callback функции для мониторинга событий
- Обмен аудио данными и DTMF сигналами
- Проверка результатов и статистики
- Graceful завершение сессий

### `examples/media_with_sdp_functional_test/simple_demo.go`
Упрощенная демонстрация основных возможностей:
- Быстрое создание двух сессий
- SDP переговоры Offer/Answer
- Проверка состояний и портов
- Базовое тестирование аудио обмена

## 🔄 Сценарий теста

### Этап 1: Инициализация
```go
// Создание менеджера с настройками localhost
config := media_with_sdp.DefaultMediaSessionWithSDPManagerConfig()
config.LocalIP = "127.0.0.1"
config.PortRange = media_with_sdp.PortRange{Min: 12000, Max: 12100}

manager, err := media_with_sdp.NewMediaSessionWithSDPManager(config)
```

### Этап 2: Создание сессий
```go
// Caller (исходящий звонок)
caller, err := manager.CreateSessionWithConfig("caller-001", callerConfig)

// Callee (входящий звонок)  
callee, err := manager.CreateSessionWithConfig("callee-002", calleeConfig)
```

### Этап 3: SDP переговоры
```go
// 1. Caller создает offer
offer, err := caller.CreateOffer()

// 2. Callee получает offer и создает answer
err = callee.SetRemoteDescription(offer)
answer, err := callee.CreateAnswer(offer)

// 3. Caller получает answer
err = caller.SetRemoteDescription(answer)
```

### Этап 4: Обмен данными
```go
// Запуск медиа сессий
err = caller.Start()
err = callee.Start()

// Обмен аудио данными
err = caller.SendAudioRaw(testAudioData)
err = callee.SendAudioRaw(testAudioData)

// Отправка DTMF сигналов
err = caller.SendDTMF(media.DTMF1, 200*time.Millisecond)
err = callee.SendDTMF(media.DTMF9, 200*time.Millisecond)
```

## ✅ Проверяемые функции

### SDP функциональность
- ✅ Создание SDP Offer с корректными медиа описаниями
- ✅ Парсинг и валидация входящего SDP
- ✅ Создание SDP Answer на основе Offer
- ✅ Установка Local/Remote descriptions
- ✅ Состояния SDP переговоров (Idle → LocalOffer → RemoteOffer → Established)

### Управление портами
- ✅ Автоматическое выделение пар портов RTP/RTCP
- ✅ Четные порты для RTP, нечетные для RTCP
- ✅ Освобождение портов при завершении сессий
- ✅ Проверка доступности портов перед выделением

### Медиа функциональность
- ✅ Композиция базовых медиа сессий
- ✅ Делегирование всех методов MediaSessionInterface
- ✅ Отправка/получение аудио данных
- ✅ Поддержка DTMF сигналов
- ✅ Thread-safe операции

### Менеджер сессий
- ✅ Создание множественных сессий
- ✅ Централизованная конфигурация
- ✅ Статистика и мониторинг
- ✅ Глобальные callback функции
- ✅ Graceful shutdown всех сессий

## 📊 Ожидаемые результаты

### SDP Offer/Answer
```
✅ SDP Offer создан (XXX байт)
📋 Медиа описаний: 1
📊 Первое медиа: audio, порт: 12000, форматы: [0 8 9]

✅ SDP Answer создан (XXX байт)
🤝 SDP переговоры завершены!
```

### Состояния переговоров
```
📊 Состояния переговоров:
   Caller: established
   Callee: established
```

### Выделенные порты
```
🔌 Выделенные порты:
   Caller: RTP=12000, RTCP=12001
   Callee: RTP=12002, RTCP=12003
```

### Статистика менеджера
```
📈 Статистика менеджера:
   Всего сессий: 2
   Активных сессий: 2
   Используемых портов: 4
```

## 🚀 Запуск тестов

### Полный функциональный тест
```bash
cd examples/media_with_sdp_functional_test
go run main.go
```

### Упрощенная демонстрация
```bash
cd examples/media_with_sdp_functional_test  
go run simple_demo.go
```

## 🔧 Техническая реализация

### Callback функции
Тест использует callback функции для мониторинга:
- `OnSessionCreated` - создание сессий
- `OnNegotiationStateChange` - изменения состояний SDP
- `OnPortsAllocated` - выделение портов
- `OnRawAudioReceived` - получение аудио данных
- `OnDTMFReceived` - получение DTMF сигналов

### Поддерживаемые кодеки
- **PCMU** (μ-law) - Payload Type 0
- **PCMA** (A-law) - Payload Type 8  
- **G722** - Payload Type 9

### Сетевые настройки
- **IP**: 127.0.0.1 (localhost)
- **Порты RTP**: 12000, 12002, 12004, ...
- **Порты RTCP**: 12001, 12003, 12005, ...
- **Протокол**: RTP/AVP

## 🎉 Заключение

Функциональный тест успешно демонстрирует:

1. **Полную интеграцию** - все компоненты работают вместе
2. **SDP совместимость** - корректный обмен SDP между сессиями
3. **Управление ресурсами** - автоматическое выделение/освобождение портов
4. **Медиа обмен** - передачу аудио данных и DTMF сигналов
5. **Масштабируемость** - возможность создания множественных сессий

Пакет `media_with_sdp` готов к использованию в реальных приложениях софтфона! 🎯 