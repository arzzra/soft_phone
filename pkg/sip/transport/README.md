# Transport Layer

Слой транспорта отвечает за сетевую передачу SIP сообщений.

## Основные компоненты

### Transport
Базовый интерфейс для всех транспортов:
- UDP Transport - ненадежный, быстрый
- TCP Transport - надежный, с соединениями
- TLS Transport - защищенный TCP

### Connection
Представляет сетевое соединение для TCP/TLS:
- Уникальный ID
- Keep-alive поддержка
- Контекст для хранения данных

### TransportManager
Управляет всеми транспортами:
- Регистрация транспортов
- Выбор транспорта по URI
- Маршрутизация сообщений

## Пример использования

```go
// Создаем менеджер транспортов
mgr := transport.NewTransportManager()

// Создаем и регистрируем UDP транспорт
udp := transport.NewUDPTransport()
err := udp.Listen("0.0.0.0:5060")
if err != nil {
    log.Fatal(err)
}
mgr.RegisterTransport(udp)

// Создаем и регистрируем TCP транспорт
tcp := transport.NewTCPTransport()
err = tcp.Listen("0.0.0.0:5060")
if err != nil {
    log.Fatal(err)
}
mgr.RegisterTransport(tcp)

// Устанавливаем обработчик сообщений
mgr.OnMessage(func(msg types.Message, addr net.Addr, transport Transport) {
    if msg.IsRequest() {
        fmt.Printf("Received %s request from %s\n", msg.Method(), addr)
    } else {
        fmt.Printf("Received %d response from %s\n", msg.StatusCode(), addr)
    }
})

// Отправляем сообщение
builder := builder.NewMessageBuilder()
msg, _ := builder.NewRequest("OPTIONS", uri).
    SetFrom(from).
    SetTo(to).
    SetCallID("test").
    SetCSeq(1, "OPTIONS").
    SetVia(via).
    Build()

// Менеджер сам выберет подходящий транспорт
err = mgr.Send(msg, "sip:bob@example.com")
```

## Особенности реализации

### UDP Transport
- Не создает соединений
- Один поток чтения
- Максимальный размер пакета 65535 байт

### TCP Transport
- Пул соединений
- Отдельный поток чтения на каждое соединение
- Парсинг SIP сообщений с учетом Content-Length
- Keep-alive поддержка

### TLS Transport
- Наследует от TCP Transport
- Минимальная версия TLS 1.2
- Поддержка пользовательской конфигурации

## Статистика

Каждый транспорт собирает статистику:
- Количество отправленных/полученных сообщений
- Количество отправленных/полученных байт
- Количество ошибок
- Активные соединения (для TCP/TLS)
