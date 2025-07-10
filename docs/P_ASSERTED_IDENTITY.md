# P-Asserted-Identity Support

## Описание

Реализована поддержка заголовка P-Asserted-Identity (RFC 3325) для SIP INVITE запросов в пакете dialog. Этот заголовок используется в доверенных SIP сетях для передачи подтвержденной идентичности вызывающего абонента.

## Новые CallOption функции

### WithAssertedIdentity(uri *sip.Uri)
Устанавливает P-Asserted-Identity заголовок с SIP URI. URI должен иметь схему `sip` или `sips`.

```go
dialog.WithAssertedIdentity(&sip.Uri{
    Scheme: "sip",
    User:   "alice",
    Host:   "example.com",
})
```

### WithAssertedIdentityFromString(identity string)
Парсит строку и устанавливает P-Asserted-Identity. Поддерживает форматы:
- `sip:user@domain`
- `sips:user@domain:port`
- `<sip:user@domain>`

```go
dialog.WithAssertedIdentityFromString("<sip:bob@example.org>")
```

### WithAssertedIdentityTel(telNumber string)
Устанавливает P-Asserted-Identity с TEL URI. Формат номера должен соответствовать E.164 (например: +1234567890).

```go
dialog.WithAssertedIdentityTel("+1234567890")
```

### WithAssertedDisplay(display string)
Устанавливает display name для P-Asserted-Identity.

```go
dialog.WithAssertedDisplay("Alice Smith")
```

### WithFromAsAssertedIdentity()
Использует From URI как P-Asserted-Identity. Удобно когда идентичность совпадает с From заголовком.

```go
dialog.WithFromAsAssertedIdentity()
```

## Примеры использования

### Пример 1: SIP URI с display name
```go
_, err := ua.CreateDialog(ctx, "target_user",
    dialog.WithAssertedIdentity(&sip.Uri{
        Scheme: "sip",
        User:   "alice",
        Host:   "example.com",
    }),
    dialog.WithAssertedDisplay("Alice Smith"),
)
// Результат: P-Asserted-Identity: "Alice Smith" <sip:alice@example.com>
```

### Пример 2: TEL URI
```go
_, err := ua.CreateDialog(ctx, "target_user",
    dialog.WithAssertedIdentityTel("+1234567890"),
    dialog.WithAssertedDisplay("John Doe"),
)
// Результат: P-Asserted-Identity: "John Doe" <tel:+1234567890>
```

### Пример 3: Множественные значения (SIP + TEL)
```go
_, err := ua.CreateDialog(ctx, "target_user",
    dialog.WithAssertedIdentity(&sip.Uri{
        Scheme: "sip",
        User:   "bob",
        Host:   "company.com",
    }),
    dialog.WithAssertedIdentityTel("+9876543210"),
    dialog.WithAssertedDisplay("Bob Wilson"),
)
// Результат:
// P-Asserted-Identity: "Bob Wilson" <sip:bob@company.com>
// P-Asserted-Identity: "Bob Wilson" <tel:+9876543210>
```

### Пример 4: Использование From как P-Asserted-Identity
```go
_, err := ua.CreateDialog(ctx, "target_user",
    dialog.WithFromUser("charlie"),
    dialog.WithFromDisplay("Charlie Brown"),
    dialog.WithFromAsAssertedIdentity(),
)
// Результат: P-Asserted-Identity: "Charlie Brown" <sip:charlie@[contact-uri-host]>
```

### Пример 5: Парсинг из строки
```go
_, err := ua.CreateDialog(ctx, "target_user",
    dialog.WithAssertedIdentityFromString("<sip:david@secure.org>"),
    dialog.WithAssertedDisplay("David Security"),
)
// Результат: P-Asserted-Identity: "David Security" <sip:david@secure.org>
```

## Технические детали

1. **Валидация схем**: Поддерживаются только схемы `sip`, `sips` и `tel`
2. **Множественные значения**: Можно добавить несколько P-Asserted-Identity заголовков (например, SIP + TEL)
3. **Display name**: Если указан `assertedDisplay`, он будет использован для всех P-Asserted-Identity заголовков
4. **Приоритет**: При использовании `WithFromAsAssertedIdentity()`:
   - Если указан `assertedDisplay` - используется он
   - Иначе используется `fromDisplay`
   - From URI используется как базовый URI

## Соответствие RFC 3325

Реализация соответствует требованиям RFC 3325:
- Поддержка SIP и TEL URI схем
- Возможность указания множественных значений
- Правильное форматирование с display name
- Валидация URI схем