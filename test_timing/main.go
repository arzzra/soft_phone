package main

import (
	"fmt"
	"log"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
)

func main() {
	fmt.Println("🎯 Тестирование правильного RTP Timing")
	fmt.Println("=====================================")

	// Создаем медиа сессию с настройками по умолчанию
	config := media.DefaultMediaSessionConfig()
	config.SessionID = "timing-test"
	config.Direction = media.DirectionSendRecv
	config.Ptime = time.Millisecond * 20 // 20ms пакеты

	session, err := media.NewMediaSession(config)
	if err != nil {
		log.Fatalf("Ошибка создания медиа сессии: %v", err)
	}
	defer session.Stop()

	// Запускаем сессию
	if err := session.Start(); err != nil {
		log.Fatalf("Ошибка запуска медиа сессии: %v", err)
	}

	fmt.Printf("✅ Медиа сессия запущена\n")
	fmt.Printf("📊 Ptime: %v\n", session.GetPtime())
	fmt.Printf("🎵 Payload Type: %s\n", session.GetPayloadType())

	// Тестируем накопление данных в буфере
	fmt.Println("\n🔄 Тестирование накопления данных в буфере:")

	// Отправляем маленькие порции данных чаще чем ptime
	for i := 0; i < 10; i++ {
		smallData := generateTestAudio(32) // Маленькие порции 32 байта

		// Используем SendAudioRaw для обхода аудио процессора
		err := session.SendAudioWithFormat(smallData, media.PayloadTypePCMU, true)
		if err != nil {
			fmt.Printf("❌ Ошибка отправки порции %d: %v\n", i+1, err)
			continue
		}

		bufferSize := session.GetBufferedAudioSize()
		timeSinceLastSend := session.GetTimeSinceLastSend()

		fmt.Printf("📦 Порция %d: +%d байт → буфер %d байт, с последней отправки %v\n",
			i+1, len(smallData), bufferSize, timeSinceLastSend.Truncate(time.Millisecond))

		time.Sleep(time.Millisecond * 5) // Отправляем каждые 5ms (чаще чем ptime 20ms)
	}

	// Ждем отправки накопленных данных
	fmt.Println("\n⏳ Ожидание автоматической отправки накопленных данных...")
	time.Sleep(time.Millisecond * 50)

	finalBufferSize := session.GetBufferedAudioSize()
	timeSinceLastSend := session.GetTimeSinceLastSend()

	fmt.Printf("📈 Итог: буфер %d байт, с последней отправки %v\n",
		finalBufferSize, timeSinceLastSend.Truncate(time.Millisecond))

	// Получаем статистику
	stats := session.GetStatistics()
	fmt.Printf("\n📊 Статистика:\n")
	fmt.Printf("   📤 Аудио пакетов отправлено: %d\n", stats.AudioPacketsSent)
	fmt.Printf("   📦 Аудио байт отправлено: %d\n", stats.AudioBytesSent)
	fmt.Printf("   🕐 Последняя активность: %v\n", stats.LastActivity.Format("15:04:05.000"))

	// Тестируем принудительную очистку буфера
	if finalBufferSize > 0 {
		fmt.Println("\n🔧 Принудительная очистка буфера...")
		err := session.FlushAudioBuffer()
		if err != nil {
			fmt.Printf("❌ Ошибка очистки буфера: %v\n", err)
		} else {
			finalBufferSize = session.GetBufferedAudioSize()
			fmt.Printf("✅ Буфер очищен, осталось: %d байт\n", finalBufferSize)
		}
	}

	fmt.Println("\n✅ Тест RTP timing завершен!")
}

// generateTestAudio генерирует тестовые аудио данные
func generateTestAudio(samples int) []byte {
	data := make([]byte, samples)
	for i := range data {
		// Генерируем простую синусоиду (G.711 μ-law симуляция)
		data[i] = byte(128 + 64*(i%32)/16)
	}
	return data
}
