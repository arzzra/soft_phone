# Конкретные требования из RFC для правильной реализации SIP

## 1. Структура SIP сообщений и заголовков (RFC 3261)

### Обязательные заголовки для запросов:
- **To**: Целевой URI (обязательно)
- **From**: Источник запроса с обязательным tag (обязательно)
- **Call-ID**: Уникальный идентификатор диалога (обязательно)
- **CSeq**: Номер последовательности и метод (обязательно)
- **Max-Forwards**: Ограничение хопов, начальное значение 70 (обязательно)
- **Via**: Путь запроса, должен содержать branch параметр (обязательно)
- **Contact**: URI для прямой связи (обязательно для INVITE, REFER)

### Обязательные заголовки для ответов:
- Все заголовки из запроса, плюс:
- **To**: Должен содержать tag в 2xx ответах для установления диалога

## 2. Машина состояний транзакций (RFC 3261 Section 17)

### Client Transaction States:
**INVITE транзакция:**
- **Calling** → **Proceeding** → **Completed** → **Terminated**
- Timer A: Ретрансмиссия INVITE (начальное значение T1=500ms, удваивается)
- Timer B: Таймаут транзакции (64*T1 = 32s)
- Timer D: Ожидание ретрансляций ACK (32s для UDP, 0s для TCP)

**Non-INVITE транзакция:**
- **Trying** → **Proceeding** → **Completed** → **Terminated**
- Timer E: Ретрансмиссия запроса (начальное значение T1)
- Timer F: Таймаут транзакции (64*T1 = 32s)
- Timer K: Ожидание ретрансляций ответа (T4=5s для UDP, 0s для TCP)

### Server Transaction States:
**INVITE транзакция:**
- **Proceeding** → **Completed** → **Confirmed** → **Terminated**
- Timer G: Ретрансмиссия ответа (начальное значение T1)
- Timer H: Ожидание ACK (64*T1)
- Timer I: Ожидание ретрансляций ACK (T4=5s для UDP, 0s для TCP)

**Non-INVITE транзакция:**
- **Trying** → **Proceeding** → **Completed** → **Terminated**
- Timer J: Ожидание ретрансляций запроса (64*T1 для UDP, 0s для TCP)

## 3. Таймеры и их значения (RFC 3261 Appendix A)

```
Timer    Value            Meaning
T1       500ms default    RTT Estimate
T2       4s               Maximum retransmit interval
T4       5s               Maximum duration in network
Timer A  initially T1     INVITE retransmit interval (UDP)
Timer B  64*T1            INVITE transaction timeout
Timer C  > 3min           Proxy INVITE timeout
Timer D  > 32s (UDP)      Wait for response retransmits
Timer E  initially T1     non-INVITE retransmit (UDP)
Timer F  64*T1            non-INVITE timeout
Timer G  initially T1     INVITE response retransmit
Timer H  64*T1            Wait for ACK receipt
Timer I  T4 (UDP)         Wait for ACK retransmits
Timer J  64*T1 (UDP)      Wait for non-INVITE retransmits
Timer K  T4 (UDP)         Wait for response retransmits
```

## 4. Dialog установка и управление (RFC 3261 Section 12)

### Создание диалога:
- **Dialog ID**: Комбинация Call-ID + local tag + remote tag
- **UAC**: Создает local tag в From заголовке
- **UAS**: Создает remote tag в To заголовке при отправке 2xx
- **Route Set**: Формируется из Record-Route заголовков

### UAC/UAS роли влияют на обработку:
```go
// UAC (User Agent Client - инициатор):
dialogKey = DialogKey{
    CallID:    callID,
    LocalTag:  fromTag,  // UAC создал этот tag
    RemoteTag: toTag,    // UAS создал этот tag
}

// UAS (User Agent Server - получатель):
dialogKey = DialogKey{
    CallID:    callID,
    LocalTag:  toTag,    // UAS создал этот tag
    RemoteTag: fromTag,  // UAC создал этот tag
}
```

## 5. REFER метод и call transfer (RFC 3515)

### Обязательные элементы REFER:
- **Refer-To заголовок**: Обязателен, содержит URI для перевода
- **Implicit subscription**: REFER автоматически создает подписку на событие "refer"
- **NOTIFY**: Обязательны для информирования о статусе

### Жизненный цикл REFER:
1. UAC отправляет REFER с Refer-To заголовком
2. UAS отвечает 202 Accepted (или отклоняет)
3. UAS создает подписку и отправляет immediate NOTIFY (100 Trying)
4. UAS выполняет действие (например, INVITE к третьей стороне)
5. UAS отправляет final NOTIFY с результатом
6. Подписка завершается с reason="noresource"

### Multiple REFER в диалоге:
- Используется id параметр в Event заголовке (CSeq номер)
- Пример: `Event: refer;id=93809824`

## 6. Обязательные заголовки и их обработка

### Via обработка:
- Каждый hop добавляет свой Via заголовок
- Branch параметр ДОЛЖЕН быть уникальным и начинаться с "z9hG4bK"
- При ответе Via заголовки удаляются в обратном порядке

### Route/Record-Route обработка:
- Record-Route добавляется прокси для остаться в пути сигнализации
- Route используется для маршрутизации последующих запросов в диалоге
- loose routing (lr параметр) vs strict routing

## 7. Критические требования для реализации

1. **Thread Safety**: Все операции с диалогами должны быть потокобезопасны
2. **Cleanup**: Автоматическая очистка завершенных транзакций и диалогов
3. **Retransmission**: Правильная обработка ретрансмиссий (особенно для UDP)
4. **State Validation**: Проверка валидности переходов состояний
5. **Timer Management**: Точное соблюдение таймеров для каждого транспорта
6. **Error Handling**: Корректная обработка ошибок и таймаутов

## 8. Специфичные требования для attended transfer

### Replaces заголовок (RFC 3891):
```
Replaces: <call-id>;to-tag=<to-tag>;from-tag=<from-tag>[;early-only]
```

### Процесс attended transfer:
1. A устанавливает вызов с B
2. A устанавливает вызов с C (consultation call)
3. A отправляет REFER к B с Refer-To содержащим C и Replaces для замены A-C диалога
4. B отправляет INVITE к C с Replaces заголовком
5. C принимает новый вызов и завершает старый с A

Эти требования критически важны для корректной реализации SIP стека, совместимого с существующими SIP устройствами и серверами.