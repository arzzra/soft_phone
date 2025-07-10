package dialog

type TransportType string

const (
	// TransportUDP - UDP транспорт
	TransportUDP TransportType = "UDP"
	// TransportTCP - TCP транспорт
	TransportTCP TransportType = "TCP"
	// TransportTLS - TLS транспорт
	TransportTLS TransportType = "TLS"
	// TransportWS - WebSocket транспорт
	TransportWS TransportType = "WS"
	// TransportWSS - WebSocket Secure транспорт
	TransportWSS TransportType = "WSS"
)

// TransportConfig содержит конфигурацию транспортного протокола.
//
// Пример использования:
//
//	config := dialog.TransportConfig{
//	    Type: dialog.TransportTLS,
//	    KeepAlive: true,
//	    KeepAlivePeriod: 30,
//	}
//
//	ua, err := dialog.NewUASUAC(
//	    dialog.WithTransport(config),
//	)
type TransportConfig struct {
	// Type - тип транспорта
	Type TransportType

	Host string

	Port int

	// TLSConfig - конфигурация TLS (для TLS и WSS)
	// TLSConfig *tls.Config // Будет добавлено при необходимости

	// WSPath - путь для WebSocket соединения (по умолчанию "/")
	WSPath string

	// KeepAlive - включить keep-alive для TCP-based транспортов
	KeepAlive bool

	// KeepAlivePeriod - период keep-alive (по умолчанию 30 секунд)
	KeepAlivePeriod int
}
