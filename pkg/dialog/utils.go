package dialog

import (
	"github.com/emiago/sipgo/sip"
)

// ParseUriString парсит строку в sip.Uri
// Удобная обёртка для использования в примерах
func ParseUriString(uriStr string) *sip.Uri {
	var uri sip.Uri
	err := sip.ParseUri(uriStr, &uri)
	if err != nil {
		return nil
	}
	return &uri
}
