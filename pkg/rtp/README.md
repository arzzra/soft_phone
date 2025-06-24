# RTP Session Package для Софтфона

Этот пакет реализует слой RTP сессий для телефонии, используя библиотеку `pion/rtp` для работы с RTP пакетами и строго следуя стандартам RFC 3550 (RTP) и RFC 3551 (RTP A/V Profile).

## Основные компоненты

### 1. Session - RTP Сессия
Основной компонент для управления RTP потоком в телефонном звонке:
- Управление SSRC (Synchronization Source ID) согласно RFC 3550
- Отслеживание sequence numbers и timestamps
- Статистика пакетов и обнаружение потерь
- Поддержка множественных источников (CSRC)
- Базовая поддержка RTCP

### 2. Transport - Интерфейс транспорта
Абстракция для различных транспортных протоколов:
- UDP транспорт (оптимизирован для голоса)
- Возможность расширения для TCP, WebRTC и др.
- Асинхронная отправка и получение пакетов

### 3. SessionManager - Менеджер сессий
Управление множественными звонками в софтфоне:
- Изоляция звонков
- Контроль ресурсов и лимитов
- Автоматическая очистка неактивных сессий
- Глобальная статистика

## Поддерживаемые Payload типы (RFC 3551)

### Аудио кодеки для телефонии:
- **PCMU (0)**: G.711 μ-law, 8kHz, 64kbps
- **PCMA (8)**: G.711 A-law, 8kHz, 64kbps  
- **GSM (3)**: GSM 06.10, 8kHz, 13kbps
- **G722 (9)**: G.722, 16kHz sampling, 64kbps
- **G728 (15)**: G.728, 8kHz, 16kbps
- **G729 (18)**: G.729, 8kHz, 8kbps

## Быстрый старт

### Создание простого звонка

```go
package main

import (
    "github.com/your-org/softphone/pkg/rtp"
)

func main() {
    // Создаем менеджер сессий
    manager := rtp.NewSessionManager(rtp.DefaultSessionManagerConfig())
    defer manager.StopAll()
    
    // Создаем UDP транспорт
    transport, err := rtp.NewUDPTransport(rtp.TransportConfig{
        LocalAddr:  ":5004",
        RemoteAddr: "192.168.1.100:5004",
        BufferSize: 1500,
    })
    if err != nil {
        panic(err)
    }
    
    // Конфигурируем RTP сессию
    sessionConfig := rtp.SessionConfig{
        PayloadType: rtp.PayloadTypePCMU, // G.711 μ-law
        MediaType:   rtp.MediaTypeAudio,
        ClockRate:   8000,
        Transport:   transport,
        LocalSDesc: rtp.SourceDescription{
            CNAME: "softphone@192.168.1.50",
            NAME:  "SIP User",
            TOOL:  "Softphone v1.0",
        },
    }
    
    // Создаем сессию
    session, err := manager.CreateSession("call-001", sessionConfig)
    if err != nil {
        panic(err)
    }
    
    // Запускаем сессию
    err = session.Start()
    if err != nil {
        panic(err)
    }
    
    // Отправляем аудио данные
    audioData := make([]byte, 160) // 20ms G.711
    err = session.SendAudio(audioData, time.Millisecond*20)
    if err != nil {
        panic(err)
    }
}
```

### Обработка входящих пакетов

```go
sessionConfig := rtp.SessionConfig{
    // ... другие настройки
    OnPacketReceived: func(packet *rtp.Packet, addr net.Addr) {
        fmt.Printf("Получен RTP: SSRC=%d, Seq=%d, TS=%d\n",
            packet.SSRC, packet.SequenceNumber, packet.Timestamp)
        
        // Обработка аудио данных
        audioData := packet.Payload
        // ... воспроизведение аудио
    },
    OnSourceAdded: func(ssrc uint32) {
        fmt.Printf("Новый источник: SSRC=%d\n", ssrc)
    },
}
```

## Архитектурные принципы

### RFC 3550 Compliance
- **Версия RTP**: Только версия 2
- **SSRC управление**: Случайная генерация с проверкой коллизий
- **Sequence Numbers**: Правильная инициализация и инкремент
- **Timestamps**: Корректное тактирование согласно payload типу
- **Статистика**: Подсчет потерь, jitter согласно RFC

### Оптимизация для телефонии
- **Низкая латентность**: Минимальные буферы, оптимизированные таймауты
- **Надежность**: Обработка сетевых ошибок и восстановление
- **Масштабируемость**: Поддержка множественных одновременных звонков
- **Ресурсы**: Контроль памяти и CPU usage

### Single Responsibility Principle
- **Session**: Только RTP поток одного звонка
- **Transport**: Только сетевая передача
- **Manager**: Только управление множественными сессиями
- **Statistics**: Только сбор и предоставление метрик

## Примеры использования

Смотрите файлы:
- `example_softphone.go` - Комплексные примеры для софтфона
- Тестовые функции для изучения API

## Зависимости

- `github.com/pion/rtp` - Для работы с RTP пакетами
- Стандартная библиотека Go

## RFC Стандарты

Реализация строго следует:
- **RFC 3550**: RTP: A Transport Protocol for Real-Time Applications
- **RFC 3551**: RTP Profile for Audio and Video Conferences with Minimal Control

## Будущие расширения

- Полная поддержка RTCP (SR, RR, SDES, BYE)
- Поддержка SRTP для безопасности
- WebRTC транспорт
- Продвинутые алгоритмы jitter buffer
- QoS метрики и адаптация битрейта 