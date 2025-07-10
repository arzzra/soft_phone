/*
Package dialog предоставляет полную реализацию управления SIP диалогами
согласно RFC 3261, включая поддержку расширенных функций переадресации (RFC 3515),
идентификации в доверенных сетях (RFC 3325) и множественных транспортных протоколов.

# Основные компоненты

Пакет состоит из следующих ключевых компонентов:

1. Dialog - представляет SIP диалог между двумя User Agent
2. UASUAC - комбинированный User Agent, выступающий в роли клиента (UAC) и сервера (UAS)
3. DialogManager - менеджер для управления коллекцией диалогов
4. TransportConfig - конфигурация транспортных протоколов (UDP, TCP, TLS, WebSocket)
5. EndpointConfig - конфигурация удалённых точек подключения с поддержкой failover

# Жизненный цикл диалога

Диалог проходит через следующие состояния:

	StateNone       → диалог не существует
	StateEarly      → ранний диалог (после получения предварительного ответа 1xx)
	StateConfirmed  → подтверждённый диалог (после получения 2xx ответа)
	StateTerminating → диалог в процессе завершения
	StateTerminated  → диалог завершён

# Базовое использование

Создание UASUAC и исходящий звонок:

	// Настройка транспорта
	transportConfig := dialog.TransportConfig{
		Type: dialog.TransportUDP,
		Host: "0.0.0.0",
		Port: 5060,
	}

	// Создаём User Agent
	ua, err := dialog.NewUASUAC(
		dialog.WithTransport(transportConfig),
		dialog.WithHostname("myphone.example.com"),
		dialog.WithContactName("alice"),
		dialog.WithLogger(logger),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer ua.Close()

	// Запускаем прослушивание входящих соединений
	ctx := context.Background()
	go ua.Listen(ctx)

	// Совершаем исходящий звонок
	dlg, err := ua.CreateDialog(ctx, "sip:alice@example.com:5060",
		dialog.WithFromUser("bob"),
		dialog.WithFromDisplay("Bob Smith"),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Ожидаем ответа и обрабатываем диалог...

# Обработка входящих вызовов

Регистрация обработчика входящих вызовов:

	manager := ua.GetDialogManager()
	manager.OnIncomingCall(func(dlg dialog.IDialog, tx sip.ServerTransaction) {
		// Принимаем звонок
		err := dlg.Answer(dialog.Body{
			ContentType: "application/sdp",
			Content:     []byte(sdpOffer),
		}, nil)
		if err != nil {
			// Отклоняем звонок
			dlg.Reject(486, "Busy Here", dialog.Body{}, nil)
		}
	})

# Конфигурация endpoints с failover

Настройка нескольких endpoints для автоматического переключения при сбоях:

	endpointConfig := &dialog.EndpointConfig{
		Primary: &dialog.Endpoint{
			Name: "main",
			Host: "sip.provider.com",
			Port: 5060,
			Transport: dialog.TransportConfig{
				Type: dialog.TransportUDP,
			},
		},
		Fallbacks: []*dialog.Endpoint{
			{
				Name: "backup1",
				Host: "sip-backup1.provider.com",
				Port: 5060,
				Transport: dialog.TransportConfig{
					Type: dialog.TransportTCP,
				},
			},
		},
	}

	// Настройка транспорта для UASUAC
	transportConfig := dialog.TransportConfig{
		Type: dialog.TransportUDP,
		Host: "192.168.1.100",
		Port: 5060,
	}

	ua, err := dialog.NewUASUAC(
		dialog.WithTransport(transportConfig),
		dialog.WithEndpoints(endpointConfig),
		dialog.WithContactName("softphone"),
	)

	// Звонок с использованием только username
	dlg, err := ua.CreateDialog(ctx, "alice") // Использует configured endpoints

# Расширенные опции вызова

Пакет поддерживает множество опций для настройки исходящих вызовов:

	opts := []dialog.CallOption{
		// Настройка идентификации
		dialog.WithFromUser("support"),
		dialog.WithFromDisplay("Техническая поддержка"),
		
		// P-Asserted-Identity для доверенных сетей
		dialog.WithAssertedIdentityFromString("sip:+79123456789@trusted.com"),
		dialog.WithAssertedDisplay("Иван Иванов"),
		
		// Дополнительные заголовки
		dialog.WithSubject("Важный звонок"),
		dialog.WithHeaders(map[string]string{
			"Priority": "emergency",
			"X-Custom": "value",
		}),
		
		// Настройка Contact для NAT
		dialog.WithContactParams(map[string]string{
			"transport": "tcp",
		}),
	}

	dlg, err := ua.CreateDialog(ctx, "alice@example.com", opts...)

# Переадресация вызовов (REFER)

Поддержка переадресации согласно RFC 3515:

	// Слепая переадресация
	tx, err := dlg.Refer(ctx, sip.Uri{
		Scheme: "sip",
		User:   "charlie",
		Host:   "example.com",
	})

	// Переадресация с заменой диалога
	tx, err := dlg.ReferReplace(ctx, anotherDialog, nil)

# Транспортные протоколы

Поддерживаются следующие транспорты:

	// UDP транспорт
	udpConfig := dialog.TransportConfig{
		Type: dialog.TransportUDP,
		Host: "0.0.0.0",
		Port: 5060,
	}

	// TCP транспорт с keep-alive
	tcpConfig := dialog.TransportConfig{
		Type:            dialog.TransportTCP,
		Host:            "0.0.0.0",
		Port:            5061,
		KeepAlive:       true,
		KeepAlivePeriod: 30,
	}

	// TLS транспорт
	tlsConfig := dialog.TransportConfig{
		Type: dialog.TransportTLS,
		Host: "0.0.0.0",
		Port: 5062,
	}

	// WebSocket транспорт
	wsConfig := dialog.TransportConfig{
		Type:   dialog.TransportWS,
		Host:   "0.0.0.0",
		Port:   8080,
		WSPath: "/sip",
	}

	// WebSocket Secure транспорт
	wssConfig := dialog.TransportConfig{
		Type:   dialog.TransportWSS,
		Host:   "0.0.0.0",
		Port:   8443,
		WSPath: "/sip",
	}

# Безопасность

Пакет включает встроенные механизмы безопасности:

- Валидация всех входящих данных
- Ограничение частоты запросов (rate limiting)
- Защита от DoS атак
- Валидация URI и заголовков
- Ограничение размеров сообщений

# Логирование

Поддерживается структурированное логирование:

	logger := dialog.NewDevelopmentLogger() // Для разработки
	logger := dialog.NewProductionLogger()  // Для продакшена

	ua, err := dialog.NewUASUAC(
		dialog.WithLogger(logger),
	)

# Соответствие стандартам

Пакет реализует следующие RFC:

- RFC 3261 - SIP: Session Initiation Protocol
- RFC 3515 - The Session Initiation Protocol (SIP) Refer Method
- RFC 3325 - Private Extensions to SIP for Asserted Identity
- RFC 3891 - The SIP "Replaces" Header
- RFC 5876 - Updates to Asserted Identity in SIP

*/
package dialog