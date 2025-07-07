package transaction

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"
)

// idCounter глобальный счетчик для генерации уникальных ID
var idCounter uint64

// GenerateTransactionID генерирует уникальный ID транзакции
func GenerateTransactionID() string {
	// Комбинируем временную метку, счетчик и случайные байты
	timestamp := time.Now().UnixNano()
	counter := atomic.AddUint64(&idCounter, 1)
	
	// Добавляем случайные байты для уникальности
	randomBytes := make([]byte, 4)
	rand.Read(randomBytes)
	
	return fmt.Sprintf("%x-%d-%s", timestamp, counter, hex.EncodeToString(randomBytes))
}