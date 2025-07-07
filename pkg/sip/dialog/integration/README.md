# Функциональные тесты SIP Dialog с OpenSIPS

Этот пакет содержит функциональные тесты для проверки функциональности SIP диалога с использованием реального OpenSIPS сервера.

## Структура

```
integration/
├── docker-compose.yml    # Конфигурация для запуска OpenSIPS и call-api
├── opensips/            # Конфигурационные файлы OpenSIPS
│   ├── opensips.cfg     # Основная конфигурация
│   └── users.txt        # База данных пользователей
├── client/              # WebSocket клиент для OpenSIPS call-api
│   ├── client.go        # Реализация клиента
│   └── client_test.go   # Юнит-тесты клиента
└── tests/               # Функциональные тесты
    ├── dialog_test.go   # Базовые тесты диалога
    └── refer_test.go    # Тесты REFER и трансфера
```

## Требования

- Docker и Docker Compose
- Go 1.19+
- Свободные порты: 5060 (SIP), 5059 (WebSocket), 8888 (MI)

## Запуск тестов

### 1. Запустите OpenSIPS и call-api

```bash
cd pkg/sip/dialog/integration
docker-compose up -d
```

Проверьте, что контейнеры запущены:
```bash
docker-compose ps
```

### 2. Запустите тесты

Все тесты:
```bash
go test ./tests/... -v
```

Только базовые тесты диалога:
```bash
go test ./tests/ -run TestBasic -v
```

Только тесты REFER:
```bash
go test ./tests/ -run TestRefer -v
go test ./tests/ -run TestTransfer -v
```

С таймаутом:
```bash
go test ./tests/... -v -timeout 60s
```

### 3. Остановите контейнеры

```bash
docker-compose down
```

## Описание тестов

### Базовые тесты диалога (dialog_test.go)

- **TestBasicCall** - установка и завершение базового звонка
- **TestCallReject** - отклонение входящего звонка
- **TestConcurrentCalls** - обработка нескольких одновременных звонков
- **TestDialogStateTransitions** - проверка переходов состояний диалога

### Тесты REFER (refer_test.go)

- **TestBlindTransfer** - слепой перевод звонка
- **TestAttendedTransfer** - перевод с консультацией
- **TestReferWithNotifications** - обработка NOTIFY после REFER
- **TestReferRejection** - отклонение запроса REFER

## Конфигурация

### OpenSIPS

Конфигурация в `opensips/opensips.cfg` включает:
- Поддержку UDP/TCP транспорта
- MI Datagram для управления через call-api
- Event Datagram для получения событий
- Dialog модуль для отслеживания звонков
- Базовую аутентификацию (отключена для тестов)

### Тестовые пользователи

Определены в `opensips/users.txt`:
- alice, bob, charlie, david (пароли: имя + 123)
- Номера: 1001, 1002, 1003

## Отладка

### Логи OpenSIPS
```bash
docker-compose logs opensips
```

### Логи call-api
```bash
docker-compose logs call-api
```

### Проверка соединения с call-api
```bash
# Установите wscat если нет
npm install -g wscat

# Подключитесь к WebSocket
wscat -c ws://localhost:5059/call-api

# Отправьте тестовый запрос
{"jsonrpc":"2.0","method":"CallStart","params":{"caller":"sip:alice@localhost","callee":"sip:bob@localhost"},"id":"1"}
```

### SIP трафик

Для отладки SIP трафика используйте:
```bash
# Запустите tcpdump в контейнере
docker exec -it opensips-test tcpdump -i any -s 0 -w - port 5060 | tcpdump -r -

# Или используйте sngrep
docker exec -it opensips-test sngrep
```

## Известные проблемы

1. **Таймауты при первом запуске** - OpenSIPS может требовать время для инициализации
2. **Порты заняты** - убедитесь что порты 5060, 5059, 8888 свободны
3. **Docker сеть** - тесты предполагают что Docker доступен на localhost

## Расширение тестов

Для добавления новых тестов:

1. Создайте новый файл в `tests/`
2. Используйте `SetupTestEnvironment()` для инициализации
3. Используйте `client.Client` для управления звонками через call-api
4. Проверяйте состояния и события диалога

Пример:
```go
func TestNewFeature(t *testing.T) {
    env, cleanup := SetupTestEnvironment(t)
    defer cleanup()
    
    ctx := context.Background()
    
    // Ваш тест
}
```