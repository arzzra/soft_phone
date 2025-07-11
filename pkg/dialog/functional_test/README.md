# Расширенные функциональные тесты для pkg/dialog

## Обзор

Этот каталог содержит расширенные функциональные тесты для пакета dialog, реализованные с использованием фреймворка testify.

## Структура тестов

### Основные тестовые файлы

1. **test_suite.go** - Базовый тестовый набор с общей функциональностью:
   - `DialogTestSuite` - основная структура для всех тестов
   - `TestEventCollector` - сбор и анализ событий
   - Вспомогательные методы для создания звонков

2. **call_transfer_test.go** - Тесты переадресации звонков (REFER):
   - Слепая переадресация (Blind Transfer)
   - Сопровождаемая переадресация (Attended Transfer)
   - Обработка ошибок переадресации
   - Множественные переадресации
   - REFER с NOTIFY уведомлениями

3. **error_handling_test.go** - Тесты обработки ошибок:
   - Таймауты (INVITE, ACK)
   - Нарушения протокола SIP
   - Состояния гонки (race conditions)
   - Исчерпание ресурсов
   - Ошибки аутентификации
   - Сетевые сбои
   - Восстановление после ошибок

4. **media_update_test.go** - Тесты обновления медиа (re-INVITE):
   - Базовый re-INVITE
   - Hold/Resume операции
   - Пересогласование кодеков
   - Добавление видео к аудио звонку
   - Изменение направления медиа потоков
   - Отклонение re-INVITE
   - Одновременные re-INVITE (glare)

5. **registration_test.go** - Тесты регистрации:
   - Базовая регистрация/дерегистрация
   - Регистрация с аутентификацией
   - Автоматическое обновление регистрации
   - Множественные регистрации
   - Переключение между регистраторами
   - Регистрация с дополнительными заголовками
   - Параллельные регистрации

### Вспомогательные файлы (helpers/)

1. **sip_helpers.go** - Утилиты для работы с SIP:
   - `SIPMessageBuilder` - построитель SIP сообщений
   - `SDPBuilder` - построитель SDP
   - Вспомогательные функции для создания SDP
   - Генераторы Call-ID, tags, branches

2. **assertions.go** - Специализированные проверки:
   - `SIPAssertions` - проверки SIP сообщений
   - `DialogAssertions` - проверки состояния диалогов
   - `TimingAssertions` - проверки таймингов
   - `MediaAssertions` - проверки медиа параметров
   - `ErrorAssertions` - проверки ошибок

## Использование

### Запуск всех тестов
```bash
go test ./pkg/dialog/functional_test/...
```

### Запуск конкретного набора тестов
```bash
# Тесты переадресации
go test -run TestCallTransferSuite ./pkg/dialog/functional_test

# Тесты обработки ошибок
go test -run TestErrorHandlingSuite ./pkg/dialog/functional_test

# Тесты медиа обновлений
go test -run TestMediaUpdateSuite ./pkg/dialog/functional_test

# Тесты регистрации
go test -run TestRegistrationSuite ./pkg/dialog/functional_test
```

### Запуск с подробным выводом
```bash
go test -v ./pkg/dialog/functional_test/...
```

### Запуск с детектором гонок
```bash
go test -race ./pkg/dialog/functional_test/...
```

### Генерация покрытия
```bash
go test -coverprofile=coverage.out ./pkg/dialog/functional_test/...
go tool cover -html=coverage.out -o coverage.html
```

## Архитектура тестов

### TestEventCollector
Централизованный сбор событий для анализа последовательности операций:
- Thread-safe операции
- Фильтрация по источнику и типу события
- Ожидание событий с таймаутом
- Проверка последовательности событий

### Базовые методы DialogTestSuite
- `SetupSuite()` - инициализация перед всеми тестами
- `SetupTest()` - подготовка перед каждым тестом
- `TearDownTest()` - очистка после каждого теста
- `initUA()` - создание User Agent
- `createBasicCall()` - установка базового звонка
- `waitForResponse()` - ожидание ответа с конкретным кодом

### Паттерны тестирования

1. **Изоляция тестов**: Каждый тест создает свои UA и транспорты
2. **Асинхронность**: Использование каналов и WaitGroup для синхронизации
3. **Таймауты**: Все ожидания имеют ограничение по времени
4. **Проверка событий**: Валидация последовательности событий
5. **Очистка ресурсов**: Автоматическое завершение диалогов

## Примеры тестовых сценариев

### Базовый звонок
```go
ua1Dialog, ua2Dialog := s.createBasicCall()
s.AssertCallEstablished()
// ... тестовая логика ...
s.TerminateCall(ua1Dialog, "UA1")
```

### Проверка последовательности событий
```go
s.AssertEventSequence("UA1", []string{
    "INVITE_SENT",
    "180_RECEIVED",
    "200_RECEIVED",
})
```

### Ожидание события с таймаутом
```go
found := s.events.WaitForEvent("UA2", "ACK_RECEIVED", 5*time.Second)
s.True(found, "Should receive ACK")
```

## Расширение тестов

Для добавления новых тестов:

1. Создайте новый файл `*_test.go` в директории functional_test
2. Определите новый TestSuite, встраивающий DialogTestSuite
3. Реализуйте тестовые методы с префиксом `Test`
4. Используйте существующие helpers и assertions
5. Добавьте функцию запуска suite в конце файла

Пример:
```go
type MyTestSuite struct {
    DialogTestSuite
}

func (s *MyTestSuite) TestMyScenario() {
    // Тестовая логика
}

func TestMySuite(t *testing.T) {
    suite.Run(t, new(MyTestSuite))
}
```

## Известные ограничения

1. Некоторые методы (Hold, OnReInvite, OnNotify, Register) могут быть не реализованы в текущей версии dialog пакета
2. Тесты используют локальные адреса (127.0.0.1) и могут требовать адаптации для других сред
3. Некоторые race conditions сложно воспроизвести детерминированно

## Рекомендации

1. Всегда проверяйте ошибки с помощью `s.Require().NoError(err)`
2. Используйте `s.events` для отслеживания последовательности операций
3. Добавляйте достаточные задержки между операциями для стабильности
4. Изолируйте тесты друг от друга, используя уникальные порты и идентификаторы
5. Документируйте сложные тестовые сценарии