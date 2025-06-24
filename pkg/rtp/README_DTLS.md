# DTLS Transport для RTP пакетов

DTLS (Datagram Transport Layer Security) транспорт обеспечивает безопасную передачу RTP пакетов для софтфонов с использованием шифрования.

## Особенности

### ✅ Основная функциональность
- **DTLS 1.2** - современный протокол безопасности для UDP
- **Безопасная передача RTP** - шифрование аудио и видео данных
- **NAT Traversal** - поддержка DTLS Connection ID для работы за NAT
- **Replay Protection** - защита от повторных атак
- **PSK Support** - поддержка pre-shared keys для IoT устройств

### 🚀 Оптимизации для VoIP
- **Низкая латентность** - оптимизированные таймауты
- **MTU 1200** - избежание фрагментации пакетов
- **Приоритизация трафика** - настройка сокетов для голоса
- **Быстрые cipher suites** - AES-GCM для максимальной производительности

### 🔒 Безопасность
- **Modern cipher suites** - ECDHE + AES-GCM
- **Extended Master Secret** - защита от протокольных атак
- **Certificate validation** - проверка сертификатов сервера
- **SRTP key export** - интеграция с SRTP для WebRTC

## Поддерживаемые режимы

### 1. DTLS с сертификатами (TLS-style)
```go
config := DefaultDTLSTransportConfig()
config.LocalAddr = "0.0.0.0:5004"
config.Certificates = []tls.Certificate{cert}

server, err := NewDTLSTransportServer(config)
```

### 2. DTLS с PSK (IoT-friendly)
```go
config := DefaultDTLSTransportConfig()
config.PSK = func(hint []byte) ([]byte, error) {
    return []byte("secret-key"), nil
}
config.CipherSuites = []dtls.CipherSuiteID{
    dtls.TLS_PSK_WITH_AES_128_GCM_SHA256,
}

client, err := NewDTLSTransportClient(config)
```

## Быстрый старт

### Создание DTLS сервера
```go
package main

import (
    "crypto/tls"
    "log"
    "your-project/pkg/rtp"
)

func main() {
    // Создаем конфигурацию
    config := rtp.DefaultDTLSTransportConfig()
    config.LocalAddr = "0.0.0.0:5004"
    config.Certificates = []tls.Certificate{yourCert}
    
    // Создаем сервер
    server, err := rtp.NewDTLSTransportServer(config)
    if err != nil {
        log.Fatal(err)
    }
    defer server.Close()
    
    log.Printf("DTLS сервер запущен на %s", server.LocalAddr())
    
    // Обработка входящих пакетов
    ctx := context.Background()
    for {
        packet, addr, err := server.Receive(ctx)
        if err != nil {
            log.Printf("Ошибка: %v", err)
            continue
        }
        
        log.Printf("Получен RTP пакет от %s", addr)
        // Обработка пакета...
    }
}
```

### Создание DTLS клиента
```go
config := rtp.DefaultDTLSTransportConfig()
config.RemoteAddr = "server.example.com:5004"
config.ServerName = "server.example.com"

client, err := rtp.NewDTLSTransportClient(config)
if err != nil {
    log.Fatal(err)
}
defer client.Close()

// Отправка RTP пакета
packet := &rtp.Packet{
    Header: rtp.Header{
        Version:        2,
        PayloadType:    8, // PCMA
        SequenceNumber: 12345,
        Timestamp:      567890,
        SSRC:           123456789,
    },
    Payload: audioData,
}

err = client.Send(packet)
```

## Конфигурация

### DTLSTransportConfig
```go
type DTLSTransportConfig struct {
    TransportConfig  // Базовые настройки (LocalAddr, RemoteAddr, BufferSize)
    
    // Сертификаты
    Certificates     []tls.Certificate
    RootCAs          *x509.CertPool
    ClientCAs        *x509.CertPool
    ServerName       string
    
    // PSK (Pre-Shared Keys)
    PSK             func([]byte) ([]byte, error)
    PSKIdentityHint []byte
    
    // Безопасность
    CipherSuites           []dtls.CipherSuiteID
    InsecureSkipVerify     bool
    
    // Производительность
    HandshakeTimeout       time.Duration  // Таймаут рукопожатия
    MTU                    int            // Размер MTU
    ReplayProtectionWindow int            // Окно защиты от replay
    EnableConnectionID     bool           // NAT traversal
}
```

### Рекомендуемые настройки

#### Для производственной среды
```go
config := DefaultDTLSTransportConfig()
config.HandshakeTimeout = 5 * time.Second
config.MTU = 1200
config.ReplayProtectionWindow = 64
config.EnableConnectionID = true
config.CipherSuites = []dtls.CipherSuiteID{
    dtls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
    dtls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
}
```

#### Для IoT устройств (PSK)
```go
config := DefaultDTLSTransportConfig()
config.PSK = yourPSKCallback
config.CipherSuites = []dtls.CipherSuiteID{
    dtls.TLS_PSK_WITH_AES_128_GCM_SHA256,
    dtls.TLS_PSK_WITH_AES_128_CCM,
}
```

## Cipher Suites

### Рекомендуемые для VoIP
| Cipher Suite | Производительность | Безопасность | Совместимость |
|-------------|-------------------|--------------|---------------|
| `TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256` | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ |
| `TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256` | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| `TLS_PSK_WITH_AES_128_GCM_SHA256` | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐ |

### Для совместимости
```go
config.CipherSuites = []dtls.CipherSuiteID{
    // Современные (приоритет)
    dtls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
    dtls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
    
    // Совместимость
    dtls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
    dtls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
}
```

## Интеграция с WebRTC

### Экспорт ключей для SRTP
```go
if client.IsHandshakeComplete() {
    // Экспортируем ключи для SRTP
    keyMaterial, err := client.ExportKeyingMaterial(
        "EXTRACTOR-dtls_srtp",
        nil,
        32,
    )
    if err != nil {
        log.Fatal(err)
    }
    
    // Используем keyMaterial для настройки SRTP
    setupSRTP(keyMaterial)
}
```

### Connection ID для NAT
```go
config.EnableConnectionID = true
// DTLS будет использовать Connection ID вместо IP:Port
// Это позволяет сессии выживать смену IP адреса
```

## Мониторинг и диагностика

### Проверка состояния соединения
```go
// Проверка установления соединения
if transport.IsHandshakeComplete() {
    state := transport.GetConnectionState()
    log.Printf("DTLS соединение активно")
    log.Printf("Сертификатов: %d", len(state.PeerCertificates))
}

// Проверка активности
if transport.IsActive() {
    log.Printf("Транспорт активен")
}
```

### Метрики производительности
```go
// Получаем информацию о cipher suite
cipherSuite := transport.GetSelectedCipherSuite()
log.Printf("Используется cipher: %v", cipherSuite)

// Экспорт метрик для мониторинга
state := transport.GetConnectionState()
// state содержит информацию о соединении
```

## Безопасность

### ✅ Лучшие практики
1. **Используйте современные cipher suites** (AES-GCM, AEAD)
2. **Включите Extended Master Secret** (по умолчанию)
3. **Не отключайте проверку сертификатов** в продакшене
4. **Используйте PSK для IoT** устройств
5. **Настройте адекватные таймауты**
6. **Регулярно обновляйте** библиотеку pion/dtls

### ⚠️ Важные предупреждения
- `InsecureSkipVerify = true` только для тестирования
- PSK должны быть криптографически стойкими
- Сертификаты должны иметь правильные SAN записи
- Connection ID раскрывает информацию о сессии

## Производительность

### Оптимизации для минимальной латентности
```go
config.HandshakeTimeout = 3 * time.Second  // Быстрое рукопожатие
config.MTU = 1200                          // Избегаем фрагментации
config.ReplayProtectionWindow = 64         // Баланс безопасности/производительности
```

### Настройка сокетов
Транспорт автоматически настраивает UDP сокеты для оптимальной работы с голосом:
- Приоритизация трафика (Linux SO_PRIORITY)
- Оптимизация буферов
- Минимизация задержек

## Примеры использования

### Базовый пример
```go
// Смотрите pkg/rtp/example_dtls.go
rtp.ExampleDTLSTransport()        // Сертификаты
rtp.ExampleDTLSTransportPSK()     // PSK режим
```

### Интеграция с медиа сессией
```go
// Создаем DTLS транспорт
dtlsTransport, err := rtp.NewDTLSTransportClient(config)
if err != nil {
    log.Fatal(err)
}

// Используем в медиа сессии
session := media.NewMediaSession(media.MediaSessionConfig{
    Transport: dtlsTransport,
    // другие настройки...
})
```

## Устранение неполадок

### Частые проблемы

#### Handshake timeout
```
Причина: Сетевые проблемы или неправильная конфигурация
Решение: Увеличить HandshakeTimeout, проверить firewall
```

#### Certificate verification failed
```
Причина: Неправильный ServerName или недоверенный сертификат
Решение: Проверить config.ServerName и config.RootCAs
```

#### PSK not found
```
Причина: PSK callback возвращает ошибку
Решение: Проверить логику PSK lookup
```

#### MTU exceeded
```
Причина: Слишком большие пакеты для сети
Решение: Уменьшить config.MTU до 1200 или меньше
```

### Отладка
```go
// Включить подробные логи (зависит от библиотеки)
config.LoggerFactory = logging.NewDefaultLoggerFactory()
```

## Сравнение с UDP транспортом

| Характеристика | UDP | DTLS |
|---------------|-----|------|
| Безопасность | ❌ Нет | ✅ Полная |
| Производительность | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ |
| Сложность настройки | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ |
| NAT traversal | ⭐⭐ | ⭐⭐⭐⭐ |
| WebRTC совместимость | ❌ | ✅ |
| IoT поддержка | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ |

## Зависимости
- `github.com/pion/dtls/v2` - DTLS 1.2 implementation
- `github.com/pion/rtp` - RTP packet handling

## Лицензия
MIT License - совместима с pion ecosystem 