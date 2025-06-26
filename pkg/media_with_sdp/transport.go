package media_with_sdp

import (
	"fmt"

	"github.com/arzzra/soft_phone/pkg/rtp"
)

// NewUDPTransport создает UDP транспорт для RTP/RTCP.
// localIP  - локальный IP-адрес (обычно 127.0.0.1 в тестах)
// port     - локальный порт для биндинга
// bufSize  - размер буфера приёма/передачи (байт)
func NewUDPTransport(localIP string, port int, bufSize int) (*rtp.UDPTransport, error) {
	cfg := rtp.TransportConfig{
		LocalAddr:  fmt.Sprintf("%s:%d", localIP, port),
		BufferSize: bufSize,
	}
	return rtp.NewUDPTransport(cfg)
}
