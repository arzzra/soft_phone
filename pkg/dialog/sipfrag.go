package dialog

import (
	"bytes"
	"strconv"
	"strings"
)

// parseSipfragStatusCode извлекает SIP status code из тела NOTIFY (message/sipfrag).
// Формат первой строки: "SIP/2.0 200 OK".
// Возвращает 0, если определить не удалось.
func parseSipfragStatusCode(body []byte) int {
	if len(body) == 0 {
		return 0
	}
	firstLine, _, _ := bytes.Cut(body, []byte("\n"))
	parts := strings.Fields(string(firstLine))
	if len(parts) < 2 {
		return 0
	}
	code, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0
	}
	return code
}
