package media_sdp

import (
	"net"
	"strconv"
)

// adjustPortInAddress увеличивает порт в адресе на указанное количество
func adjustPortInAddress(addr string, offset int) (string, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", err
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", err
	}

	newPort := port + offset
	return net.JoinHostPort(host, strconv.Itoa(newPort)), nil
}
