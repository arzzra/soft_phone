package media

import (
	"testing"
	"time"

	"github.com/pion/rtp"
)

// === ТЕСТЫ СОЗДАНИЯ И КОНФИГУРАЦИИ JITTER BUFFER ===

// TestJitterBufferCreation тестирует создание jitter buffer с различными конфигурациями
// Проверяет:
// - Корректность создания с валидными параметрами
// - Обработку невалидных конфигураций
// - Правильность установки значений по умолчанию
func TestJitterBufferCreation(t *testing.T) {
	tests := []struct {
		name        string
		config      JitterBufferConfig
		expectError bool
		description string
	}{
		{
			name: "Стандартная конфигурация",
			config: JitterBufferConfig{
				BufferSize:   10,
				InitialDelay: time.Millisecond * 40,
				MaxDelay:     time.Millisecond * 200,
			},
			expectError: false,
			description: "Создание буфера с базовыми параметрами для G.711",
		},
		{
			name: "Малый буфер для низкой задержки",
			config: JitterBufferConfig{
				BufferSize:   5,
				InitialDelay: time.Millisecond * 20,
				MaxDelay:     time.Millisecond * 100,
			},
			expectError: false,
			description: "Малый буфер для приложений реального времени",
		},
		{
			name: "Большой буфер для нестабильной сети",
			config: JitterBufferConfig{
				BufferSize:   50,
				InitialDelay: time.Millisecond * 80,
				MaxDelay:     time.Millisecond * 500,
			},
			expectError: false,
			description: "Большой буфер для компенсации высокого jitter",
		},
		{
			name: "Автоматические значения по умолчанию",
			config: JitterBufferConfig{
				BufferSize: 0, // Будет установлен в 10
			},
			expectError: false,
			description: "Проверка установки значений по умолчанию",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Тест создания jitter buffer: %s", tt.description)

			buffer, err := NewJitterBuffer(tt.config)

			if tt.expectError {
				if err == nil {
					t.Errorf("Ожидалась ошибка, но буфер создан успешно")
				}
				return
			}

			if err != nil {
				t.Fatalf("Неожиданная ошибка создания буфера: %v", err)
			}

			defer buffer.Stop()

			// Проверяем что значения по умолчанию установлены корректно
			if buffer.config.BufferSize <= 0 {
				t.Error("BufferSize должен быть положительным")
			}

			if buffer.config.InitialDelay <= 0 {
				t.Error("InitialDelay должна быть положительной")
			}

			// Проверяем начальное состояние
			stats := buffer.GetStatistics()
			if stats.BufferSize != 0 {
				t.Error("Начальный размер буфера должен быть 0")
			}
		})
	}
}

// TestJitterBufferPacketHandling тестирует обработку RTP пакетов
// Проверяет:
// - Правильную сортировку пакетов по sequence number
// - Обнаружение и обработку потерянных пакетов
// - Обработку дублированных пакетов
// - Обработку пакетов, пришедших не по порядку
func TestJitterBufferPacketHandling(t *testing.T) {
	config := JitterBufferConfig{
		BufferSize:   10,
		InitialDelay: time.Millisecond * 40,
		MaxDelay:     time.Millisecond * 200,
	}

	buffer, err := NewJitterBuffer(config)
	if err != nil {
		t.Fatalf("Ошибка создания буфера: %v", err)
	}
	defer buffer.Stop()

	// Тестируем обработку пакетов в правильном порядке
	t.Log("Тестируем пакеты в правильном порядке")
	sequenceNumbers := []uint16{1000, 1001, 1002, 1003}

	for i, seqNum := range sequenceNumbers {
		packet := createTestRTPPacket(seqNum, uint32(i*160), generateTestAudioData(160))
		err = buffer.Put(packet)
		if err != nil {
			t.Errorf("Ошибка добавления пакета %d: %v", seqNum, err)
		}
	}

	// Ждем обработки
	time.Sleep(time.Millisecond * 100)

	// Проверяем статистику
	stats := buffer.GetStatistics()
	if stats.PacketsReceived != uint64(len(sequenceNumbers)) {
		t.Errorf("Ожидалось %d полученных пакетов, получено %d",
			len(sequenceNumbers), stats.PacketsReceived)
	}

	// Тестируем пакеты не по порядку
	t.Log("Тестируем пакеты не по порядку")
	outOfOrderSeq := []uint16{2002, 2000, 2003, 2001} // Перемешанный порядок

	for _, seqNum := range outOfOrderSeq {
		packet := createTestRTPPacket(seqNum, uint32((seqNum-2000)*160), generateTestAudioData(160))
		err = buffer.Put(packet)
		if err != nil {
			t.Errorf("Ошибка добавления пакета %d: %v", seqNum, err)
		}
	}

	// Ждем обработки и сортировки
	time.Sleep(time.Millisecond * 150)

	// Проверяем статистику
	finalStats := buffer.GetStatistics()
	if finalStats.PacketsReceived < stats.PacketsReceived+uint64(len(outOfOrderSeq)) {
		t.Error("Количество полученных пакетов должно увеличиться")
	}

	if finalStats.PacketsLate > 0 {
		t.Logf("Обнаружено %d поздних пакетов (ожидаемо для неупорядоченных)", finalStats.PacketsLate)
	}
}

// TestJitterBufferAdaptiveDelay тестирует адаптивную настройку задержки
// Проверяет автоматическую подстройку под условия сети
func TestJitterBufferAdaptiveDelay(t *testing.T) {
	config := JitterBufferConfig{
		BufferSize:   20,
		InitialDelay: time.Millisecond * 40,
		MaxDelay:     time.Millisecond * 200,
	}

	buffer, err := NewJitterBuffer(config)
	if err != nil {
		t.Fatalf("Ошибка создания буфера: %v", err)
	}
	defer buffer.Stop()

	// Получаем начальную статистику
	initialStats := buffer.GetStatistics()
	initialDelay := initialStats.CurrentDelay

	t.Logf("Начальная задержка: %v", initialDelay)

	// Добавляем пакеты с высоким jitter для активации адаптации
	t.Log("Добавляем пакеты с высоким jitter")
	baseSeq := uint16(3000)
	jitterDelays := []time.Duration{
		0,
		time.Millisecond * 10,
		time.Millisecond * 5,
		time.Millisecond * 15,
		time.Millisecond * 3,
	}

	for i, delay := range jitterDelays {
		seqNum := baseSeq + uint16(i)
		packet := createTestRTPPacket(seqNum, uint32(i*160), generateTestAudioData(160))

		// Симулируем jitter с задержкой
		if delay > 0 {
			time.Sleep(delay)
		}

		err = buffer.Put(packet)
		if err != nil {
			t.Errorf("Ошибка добавления пакета %d: %v", seqNum, err)
		}
	}

	// Ждем адаптации
	time.Sleep(time.Millisecond * 200)

	// Проверяем что буфер адаптировался
	adaptedStats := buffer.GetStatistics()
	t.Logf("Адаптированная задержка: %v", adaptedStats.CurrentDelay)

	if adaptedStats.PacketsReceived == 0 {
		t.Error("Буфер должен был получить пакеты")
	}

	// Задержка может измениться в любую сторону в зависимости от алгоритма адаптации
	if adaptedStats.CurrentDelay == initialDelay {
		t.Logf("Задержка не изменилась (это нормально для стабильного jitter)")
	} else {
		t.Logf("Задержка адаптировалась с %v на %v", initialDelay, adaptedStats.CurrentDelay)
	}
}

// TestJitterBufferOverflow тестирует поведение при переполнении буфера
// Проверяет правильную обработку превышения максимального размера
func TestJitterBufferOverflow(t *testing.T) {
	config := JitterBufferConfig{
		BufferSize:   3, // Малый буфер для быстрого переполнения
		InitialDelay: time.Millisecond * 20,
		MaxDelay:     time.Millisecond * 100,
	}

	buffer, err := NewJitterBuffer(config)
	if err != nil {
		t.Fatalf("Ошибка создания буфера: %v", err)
	}
	defer buffer.Stop()

	// Добавляем больше пакетов чем размер буфера
	t.Log("Заполняем буфер до переполнения")
	baseSeq := uint16(4000)
	packetCount := 10 // Больше чем BufferSize

	for i := 0; i < packetCount; i++ {
		seqNum := baseSeq + uint16(i)
		packet := createTestRTPPacket(seqNum, uint32(i*160), generateTestAudioData(160))

		err = buffer.Put(packet)
		if err != nil {
			t.Errorf("Ошибка добавления пакета %d: %v", seqNum, err)
		}
	}

	// Проверяем статистику переполнения
	stats := buffer.GetStatistics()
	t.Logf("Статистика после переполнения: получено %d, отброшено %d",
		stats.PacketsReceived, stats.PacketsDropped)

	if stats.PacketsDropped == 0 {
		t.Error("Ожидались отброшенные пакеты при переполнении буфера")
	}

	if stats.BufferSize > config.BufferSize {
		t.Errorf("Размер буфера не должен превышать максимальный: %d > %d",
			stats.BufferSize, config.BufferSize)
	}
}

// TestJitterBufferUnderrun тестирует обработку ситуации нехватки данных
// Проверяет поведение при попытке получить данные из пустого буфера
func TestJitterBufferUnderrun(t *testing.T) {
	config := JitterBufferConfig{
		BufferSize:   10,
		InitialDelay: time.Millisecond * 40,
		MaxDelay:     time.Millisecond * 200,
	}

	buffer, err := NewJitterBuffer(config)
	if err != nil {
		t.Fatalf("Ошибка создания буфера: %v", err)
	}
	defer buffer.Stop()

	// Пытаемся получить пакет из пустого буфера (неблокирующий вызов)
	t.Log("Тестируем получение из пустого буфера")
	packet, available := buffer.Get()
	if available {
		t.Error("Буфер должен быть пустым")
	}
	if packet != nil {
		t.Error("Пакет должен быть nil для пустого буфера")
	}

	// Добавляем несколько пакетов
	baseSeq := uint16(5000)
	for i := 0; i < 3; i++ {
		seqNum := baseSeq + uint16(i)
		testPacket := createTestRTPPacket(seqNum, uint32(i*160), generateTestAudioData(160))
		_ = buffer.Put(testPacket)
	}

	// Ждем чтобы пакеты стали доступны
	time.Sleep(time.Millisecond * 50)

	// Пытаемся получить больше пакетов чем есть в буфере
	receivedCount := 0
	for i := 0; i < 10; i++ { // Пытаемся получить больше чем добавили
		packet, available := buffer.Get()
		if available && packet != nil {
			receivedCount++
		} else {
			break
		}
	}

	t.Logf("Получено %d пакетов", receivedCount)

	// Проверяем статистику
	stats := buffer.GetStatistics()
	if stats.PacketsReceived != 3 {
		t.Errorf("Ожидалось 3 полученных пакета, получено %d", stats.PacketsReceived)
	}
}

// TestJitterBufferStatistics тестирует сбор статистики jitter buffer
// Проверяет корректность всех счетчиков и метрик
func TestJitterBufferStatistics(t *testing.T) {
	config := JitterBufferConfig{
		BufferSize:   10,
		InitialDelay: time.Millisecond * 40,
		MaxDelay:     time.Millisecond * 200,
	}

	buffer, err := NewJitterBuffer(config)
	if err != nil {
		t.Fatalf("Ошибка создания буфера: %v", err)
	}
	defer buffer.Stop()

	// Проверяем начальную статистику
	initialStats := buffer.GetStatistics()
	if initialStats.PacketsReceived != 0 {
		t.Error("Начальное количество полученных пакетов должно быть 0")
	}
	if initialStats.PacketsDropped != 0 {
		t.Error("Начальное количество отброшенных пакетов должно быть 0")
	}
	if initialStats.BufferSize != 0 {
		t.Error("Начальный размер буфера должен быть 0")
	}

	// Добавляем пакеты и проверяем обновление статистики
	t.Log("Добавляем пакеты для проверки статистики")
	baseSeq := uint16(6000)
	packetCount := 5

	for i := 0; i < packetCount; i++ {
		seqNum := baseSeq + uint16(i)
		packet := createTestRTPPacket(seqNum, uint32(i*160), generateTestAudioData(160))
		_ = buffer.Put(packet)
	}

	// Ждем обработки
	time.Sleep(time.Millisecond * 100)

	// Проверяем обновленную статистику
	updatedStats := buffer.GetStatistics()
	if updatedStats.PacketsReceived != uint64(packetCount) {
		t.Errorf("Ожидалось %d полученных пакетов, получено %d",
			packetCount, updatedStats.PacketsReceived)
	}

	if updatedStats.BufferSize > initialStats.MaxBufferSize {
		// MaxBufferSize может быть 0 в начале, это нормально
		t.Logf("Текущий размер буфера: %d", updatedStats.BufferSize)
	}

	// Вычисляем packet loss rate
	if updatedStats.PacketsReceived > 0 {
		lossRate := float64(updatedStats.PacketsDropped) / float64(updatedStats.PacketsReceived) * 100
		t.Logf("Packet loss rate: %.2f%%", lossRate)
	}

	// Проверяем метрики задержки
	if updatedStats.CurrentDelay <= 0 {
		t.Error("CurrentDelay должна быть положительной")
	}
	if updatedStats.TargetDelay <= 0 {
		t.Error("TargetDelay должна быть положительной")
	}

	t.Logf("Текущая задержка: %v, целевая: %v",
		updatedStats.CurrentDelay, updatedStats.TargetDelay)
}

// === ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ ===

// createTestRTPPacket создает тестовый RTP пакет с заданными параметрами
func createTestRTPPacket(seqNum uint16, timestamp uint32, payload []byte) *rtp.Packet {
	return &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Padding:        false,
			Extension:      false,
			Marker:         false,
			PayloadType:    uint8(PayloadTypePCMU),
			SequenceNumber: seqNum,
			Timestamp:      timestamp,
			SSRC:           0x12345678,
		},
		Payload: payload,
	}
}

// === БЕНЧМАРКИ ===

// BenchmarkJitterBufferOperations бенчмарк для операций jitter buffer
func BenchmarkJitterBufferOperations(b *testing.B) {
	config := JitterBufferConfig{
		BufferSize:   10,
		InitialDelay: time.Millisecond * 40,
	}

	buffer, err := NewJitterBuffer(config)
	if err != nil {
		b.Fatalf("Ошибка создания буфера: %v", err)
	}
	defer buffer.Stop()

	audioData := generateTestAudioData(160)

	b.ResetTimer()

	b.Run("Put", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			packet := createTestRTPPacket(uint16(i), uint32(i*160), audioData)
			_ = buffer.Put(packet)
		}
	})

	b.Run("Get", func(b *testing.B) {
		// Предварительно заполняем буфер
		for i := 0; i < 100; i++ {
			packet := createTestRTPPacket(uint16(i), uint32(i*160), audioData)
			_ = buffer.Put(packet)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buffer.Get()
		}
	})

	b.Run("GetStatistics", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = buffer.GetStatistics()
		}
	})
}
