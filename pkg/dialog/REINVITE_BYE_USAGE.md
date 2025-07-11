# Использование методов ReInvite и Bye

Этот документ описывает использование методов `ReInvite` и `Bye` для управления SIP диалогами.

## ReInvite

Метод `ReInvite` используется для изменения параметров существующего вызова. Это может включать:
- Изменение кодеков
- Постановку вызова на hold
- Добавление/удаление медиа потоков (например, видео)
- Изменение направления медиа (sendonly, recvonly, sendrecv)

### Сигнатура метода

```go
ReInvite(ctx context.Context, opts ...RequestOpt) (IClientTX, error)
```

### Важные особенности

1. **Состояние диалога**: ReInvite может быть отправлен только в состоянии `InCall`
2. **Параметр target удален**: В отличие от обычного INVITE, re-INVITE не требует указания цели, так как отправляется в рамках существующего диалога
3. **Транзакция сохраняется**: Транзакция re-INVITE сохраняется в диалоге для отслеживания

### Примеры использования

#### Изменение кодеков
```go
newSDP := `v=0
o=alice 123 124 IN IP4 192.168.1.100
s=Session
c=IN IP4 192.168.1.100
t=0 0
m=audio 5004 RTP/AVP 0 8 18
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000
a=rtpmap:18 G729/8000`

tx, err := dialog.ReInvite(ctx, 
    dialog.WithSDP(newSDP),
    dialog.WithHeaderString("Subject", "Codec update"))
if err != nil {
    log.Printf("Failed to send re-INVITE: %v", err)
    return
}

// Ожидание ответа
select {
case resp := <-tx.Responses():
    if resp.StatusCode == 200 {
        log.Println("re-INVITE accepted")
    }
case <-ctx.Done():
    log.Println("Context cancelled")
}
```

#### Постановка на hold
```go
holdSDP := `v=0
o=alice 123 125 IN IP4 192.168.1.100
s=Session
c=IN IP4 192.168.1.100
t=0 0
m=audio 5004 RTP/AVP 0
a=rtpmap:0 PCMU/8000
a=sendonly`

tx, err := dialog.ReInvite(ctx, dialog.WithSDP(holdSDP))
```

#### Возобновление после hold
```go
resumeSDP := `v=0
o=alice 123 126 IN IP4 192.168.1.100
s=Session
c=IN IP4 192.168.1.100
t=0 0
m=audio 5004 RTP/AVP 0
a=rtpmap:0 PCMU/8000
a=sendrecv`

tx, err := dialog.ReInvite(ctx, dialog.WithSDP(resumeSDP))
```

### Обработка входящих re-INVITE

```go
dialog.OnRequestHandler(func(tx IServerTX) {
    req := tx.Request()
    if req.Method == sip.INVITE && req.To().Params.Has("tag") {
        // Это re-INVITE
        log.Println("Received re-INVITE")
        
        // Анализ новых параметров
        if body := req.Body(); body != nil {
            // Обработка SDP
        }
        
        // Принять re-INVITE
        err := tx.Accept()
        if err != nil {
            // Отклонить если не можем принять новые параметры
            tx.Reject(488, "Not Acceptable Here")
        }
    }
})
```

## Bye

Метод `Bye` используется для завершения установленного вызова путем отправки SIP BYE запроса.

### Сигнатура метода

```go
Bye(ctx context.Context) error
```

### Важные особенности

1. **Состояние диалога**: BYE может быть отправлен только в состоянии `InCall`
2. **Ожидание ответа**: Метод ожидает ответа на BYE перед завершением
3. **Изменение состояния**: Диалог переходит в состояние `Terminating`, затем в `Ended`
4. **Альтернатива Terminate()**: Метод `Bye` является альтернативой существующему методу `Terminate()`

### Примеры использования

#### Базовое завершение вызова
```go
if dialog.State() == dialog.InCall {
    err := dialog.Bye(ctx)
    if err != nil {
        log.Printf("Failed to send BYE: %v", err)
    }
}
```

#### Завершение с таймаутом
```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

err := dialog.Bye(ctx)
if err != nil {
    if err == context.DeadlineExceeded {
        log.Println("BYE timeout")
    } else {
        log.Printf("BYE failed: %v", err)
    }
}
```

### Обработка входящих BYE

```go
dialog.OnBye(func(d IDialog, tx IServerTX) {
    log.Println("Received BYE request")
    
    // Отправить 200 OK
    err := tx.Accept()
    if err != nil {
        log.Printf("Failed to respond to BYE: %v", err)
    }
    
    // Освободить ресурсы
    // ...
})
```

## Различия между Terminate() и Bye()

| Функция | Terminate() | Bye() |
|---------|-------------|-------|
| Назначение | Общее завершение диалога | Отправка BYE запроса |
| Параметры | Нет | Context для отмены/таймаута |
| Возвращаемое значение | error | error |
| Ожидание ответа | Нет | Да |
| Использование | Упрощенное API | Больше контроля |

## Обработка ошибок

### ReInvite
- **Неправильное состояние**: "re-INVITE разрешен только в состоянии InCall"
- **Сетевые ошибки**: "не удалось отправить re-INVITE"
- **Таймаут**: Используйте контекст с таймаутом

### Bye
- **Неправильное состояние**: "BYE может быть отправлен только в состоянии InCall"
- **Сетевые ошибки**: "не удалось отправить BYE"
- **Таймаут ответа**: "context deadline exceeded"

## Рекомендации

1. **Всегда проверяйте состояние диалога** перед отправкой ReInvite или Bye
2. **Используйте контекст с таймаутом** для предотвращения зависаний
3. **Обрабатывайте отклонения re-INVITE** - удаленная сторона может не принять новые параметры
4. **Освобождайте ресурсы** после получения/отправки BYE
5. **Логируйте все операции** для отладки
6. **Не отправляйте множественные re-INVITE одновременно** - дождитесь ответа на предыдущий

## Тестирование

Для тестирования функциональности ReInvite и Bye рекомендуется:

1. Использовать SIP тестовые инструменты (SIPp, PJSUA)
2. Создавать интеграционные тесты с реальными транспортами
3. Проверять различные сценарии отказов
4. Тестировать параллельные операции

См. примеры в файле `examples/reinvite_bye_example.go`.