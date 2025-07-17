package rtp

// Direction определяет направление медиа потока
type Direction int

const (
	DirectionSendRecv Direction = iota // Отправка и прием
	DirectionSendOnly                  // Только отправка
	DirectionRecvOnly                  // Только прием
	DirectionInactive                  // Неактивно
)

func (d Direction) String() string {
	switch d {
	case DirectionSendRecv:
		return "sendrecv"
	case DirectionSendOnly:
		return "sendonly"
	case DirectionRecvOnly:
		return "recvonly"
	case DirectionInactive:
		return "inactive"
	default:
		return "unknown"
	}
}

// CanSend проверяет, может ли поток отправлять данные
func (d Direction) CanSend() bool {
	return d == DirectionSendRecv || d == DirectionSendOnly
}

// CanReceive проверяет, может ли поток принимать данные
func (d Direction) CanReceive() bool {
	return d == DirectionSendRecv || d == DirectionRecvOnly
}
