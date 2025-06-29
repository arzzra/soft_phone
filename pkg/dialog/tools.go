package dialog

import (
	"crypto/rand"
	"encoding/hex"
)

func TagGen() string {
	return generateTag()
}

// generateTag генерирует случайный тег для диалога
func generateTag() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
