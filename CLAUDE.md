# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Important Context
- IMPORTANT: Отвечай и пиши комментарии и документацию на русском языке
- Be brutally honest, don't be a yes man. If I am wrong, point it out bluntly.
- I need honest feedback on my code.

## Code Quality
**ВАЖНО**: После внесения изменений в код необходимо:
1. Запустить линтер командой `golangci-lint run` (глобального Makefile нет)
2. Исправить все обнаруженные линтером ошибки и предупреждения
3. Только после успешного прохождения линтера считать задачу выполненной
4. Сделать commit полноценный. Не указывай себя в авторах

## Common Development Commands

### Building and Running
```bash
# Сборка всего проекта
go build ./...

# Запуск конкретного исполняемого файла
go run ./cmd/test_dtls/main.go

# Сборка с указанием выходного файла
go build -o bin/softphone ./cmd/test_dtls
```

### Testing
```bash
# Запуск всех тестов
go test ./...

# Запуск тестов с подробным выводом
go test -v ./...

# Запуск тестов с детектором гонок
go test -race ./...

# Запуск конкретного теста
go test ./pkg/dialog -run TestSpecificFunction

# Генерация покрытия кода
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

### Linting and Formatting
```bash
# Установка golangci-lint (если не установлен)
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Запуск линтера
golangci-lint run

# Форматирование кода
go fmt ./...

# Проверка кода на потенциальные ошибки
go vet ./...
```

## High-Level Architecture

### Package Structure
```
soft_phone/
├── pkg/dialog/      # SIP протокол и управление диалогами
├── pkg/media/       # Высокоуровневая обработка медиа (аудио, кодеки, jitter buffer)
├── pkg/rtp/         # Низкоуровневая работа с RTP/RTCP транспортом
└── pkg/media_sdp/   # SDP обработка и интеграция с медиа слоем
```

### Архитектурные связи
1. **pkg/dialog** обрабатывает SIP сигнализацию и управляет жизненным циклом звонков
2. **pkg/media_sdp** служит мостом между SIP (SDP) и медиа обработкой
3. **pkg/media** предоставляет высокоуровневый API для работы с аудио потоками
4. **pkg/rtp** обеспечивает низкоуровневую передачу RTP/RTCP пакетов

### Основные интерфейсы
- **IDialog** (pkg/dialog) - управление SIP диалогом
- **IUU** (pkg/dialog) - менеджер диалогов (UAC/UAS)
- **MediaSession** (pkg/media) - управление медиа потоками
- **SDPMediaHandler** (pkg/media_sdp) - обработка SDP offer/answer
- **Session** (pkg/rtp) - RTP сессия

### Поток обработки вызова
1. SIP INVITE принимается в **pkg/dialog**
2. SDP offer передается в **pkg/media_sdp** для парсинга
3. **pkg/media_sdp** создает RTP транспорт через **pkg/rtp**
4. **pkg/media_sdp** создает медиа сессию через **pkg/media**
5. SDP answer генерируется и отправляется через **pkg/dialog**
6. Начинается обмен RTP пакетами

## Supported Features
- **Транспорты**: UDP, TCP, TLS (TODO), WS, WSS (TODO)
- **Кодеки**: G.711 (μ-law/A-law), G.722, GSM, G.728, G.729
- **DTMF**: RFC 4733 (телефонные события)
- **Jitter Buffer**: Адаптивная компенсация сетевого джиттера
- **RTCP**: Отчеты о качестве связи

# Правила использования MCP инструментов

## Общие принципы
- **ПРИОРИТЕТ**: Всегда используй MCP инструменты вместо стандартных поисковых инструментов для Go кода
- **ЭФФЕКТИВНОСТЬ**: MCP инструменты предоставляют более точную и структурированную информацию
- **ПОСЛЕДОВАТЕЛЬНОСТЬ**: Используй инструменты в правильном порядке для максимальной эффективности

## 1. MCP gopls - Навигация и анализ Go кода

### Когда использовать:
- **ВСЕГДА** для навигации по Go коду
- При поиске определений функций, типов, переменных
- Для поиска всех реализаций интерфейса
- При рефакторинге (переименование символов)
- Для поиска всех использований символа
- При анализе структуры кода
- Для получения информации о символах (hover)

### Приоритет инструментов gopls:
1. **mcp__mcp-gopls__GoToDefinition** - переход к определению
2. **mcp__mcp-gopls__FindReferences** - поиск всех использований
3. **mcp__mcp-gopls__FindImplementers** - поиск реализаций интерфейса
4. **mcp__mcp-gopls__Hover** - информация о символе
5. **mcp__mcp-gopls__ListDocumentSymbols** - структура файла
6. **mcp__mcp-gopls__SearchSymbol** - поиск символов по имени
7. **mcp__mcp-gopls__RenameSymbol** - переименование
8. **mcp__mcp-gopls__GetDiagnostics** - ошибки и предупреждения
9. **mcp__mcp-gopls__FormatCode** - форматирование кода
10. **mcp__mcp-gopls__OrganizeImports** - организация импортов

### Примеры использования:
```
// Поиск определения функции
mcp__mcp-gopls__GoToDefinition(file, line, column)

// Поиск всех использований переменной
mcp__mcp-gopls__FindReferences(file, line, column)

// Поиск всех реализаций интерфейса
mcp__mcp-gopls__FindImplementers(file, line, column)
```

## 2. MCP godoc - Документация Go пакетов

### Когда использовать:
- **ВСЕГДА** перед чтением исходного кода пакета
- При изучении API стандартной библиотеки
- Для понимания внешних зависимостей
- При работе с локальными пакетами
- Для получения примеров использования

### Приоритет использования:
1. **Сначала** - базовая документация пакета
2. **Затем** - конкретные символы (функции, типы)
3. **При необходимости** - исходный код с флагом -src

### Примеры использования:
```
// Базовая документация пакета
mcp__godoc-mcp__get_doc(path="net/http")

// Документация конкретной функции
mcp__godoc-mcp__get_doc(path="net/http", target="HandleFunc")

// Полная документация с примерами
mcp__godoc-mcp__get_doc(path="io", cmd_flags=["-all"])
```

## 3. MCP context7 - Документация библиотек

### Когда использовать:
- При работе с внешними библиотеками (не Go)
- Для изучения фреймворков и инструментов
- При поиске примеров использования
- Для получения актуальной документации

### Процесс работы:
1. **Всегда** сначала вызывай `resolve-library-id`
2. **Затем** используй полученный ID для `get-library-docs`

### Примеры использования:
```
// Поиск ID библиотеки
mcp__context7__resolve-library-id(libraryName="mongodb")

// Получение документации
mcp__context7__get-library-docs(context7CompatibleLibraryID="/mongodb/docs")
```

## 4. MCP ide - Диагностика IDE

### Когда использовать:
- Для получения диагностической информации
- При отладке проблем с IDE
- Дополнительно к gopls диагностике

## Workflow для работы с Go кодом:

### 1. Изучение нового кода:
```
1. mcp__mcp-gopls__ListDocumentSymbols - структура файла
2. mcp__godoc-mcp__get_doc - документация пакета
3. mcp__mcp-gopls__Hover - детали по символам
4. mcp__mcp-gopls__GoToDefinition - переход к определениям
```

### 2. Поиск функциональности:
```
1. mcp__mcp-gopls__SearchSymbol - поиск по имени
2. mcp__godoc-mcp__get_doc - проверка документации
3. mcp__mcp-gopls__FindReferences - где используется
```

### 3. Рефакторинг:
```
1. mcp__mcp-gopls__FindReferences - найти все использования
2. mcp__mcp-gopls__RenameSymbol - переименовать
3. mcp__mcp-gopls__GetDiagnostics - проверить ошибки
4. mcp__mcp-gopls__FormatCode - форматировать код
```

### 4. Анализ интерфейсов:
```
1. mcp__mcp-gopls__GoToDefinition - найти интерфейс
2. mcp__mcp-gopls__FindImplementers - найти реализации
3. mcp__godoc-mcp__get_doc - документация интерфейса
```

## Важные правила:

1**ВСЕГДА** начинай с MCP инструментов
2**Используй** gopls для навигации и анализа
3**Используй** godoc для документации перед чтением кода
4**Комбинируй** инструменты для максимальной эффективности

## Приоритет инструментов (от высокого к низкому):
1. MCP gopls - для всей работы с Go кодом
2. MCP godoc - для документации Go пакетов
3. MCP context7 - для внешних библиотек
4. MCP ide - для диагностики
5. Стандартные инструменты - только если MCP недоступен