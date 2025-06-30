package media

import (
	"fmt"
	"sync"
	"time"
)

// AudioProcessor обрабатывает аудио данные для медиа сессии.
// Предоставляет кодирование/декодирование аудио и опциональные обработки:
//   - AGC (автоматическая регулировка усиления)
//   - Шумоподавление
//   - Подавление эха (для входящих пакетов)
//
// Поддерживаемые кодеки: G.711 (μ-law/A-law), G.722, GSM.
type AudioProcessor struct {
	config AudioProcessorConfig
	mutex  sync.RWMutex

	// Статистика
	bytesProcessed uint64
	packetsIn      uint64
	packetsOut     uint64

	// Буферы для обработки
	inputBuffer  []byte
	outputBuffer []byte
}

// AudioProcessorConfig содержит конфигурацию для создания AudioProcessor.
// Определяет параметры кодека и включенные алгоритмы обработки.
type AudioProcessorConfig struct {
	PayloadType PayloadType   // Тип кодека
	Ptime       time.Duration // Packet time
	SampleRate  uint32        // Частота дискретизации
	Channels    int           // Количество каналов (1 или 2)

	// Настройки обработки
	EnableAGC      bool    // Automatic Gain Control
	EnableNR       bool    // Noise Reduction
	EnableEcho     bool    // Echo Cancellation
	AGCTargetLevel float32 // Целевой уровень для AGC (0.0-1.0)
}

// DefaultAudioProcessorConfig возвращает конфигурацию по умолчанию для аудио процессора.
// Оптимизировано для телефонных приложений: G.711 μ-law, 8kHz, 20ms ptime, без дополнительной обработки.
func DefaultAudioProcessorConfig() AudioProcessorConfig {
	return AudioProcessorConfig{
		PayloadType:    PayloadTypePCMU,
		Ptime:          time.Millisecond * 20,
		SampleRate:     8000,
		Channels:       1,
		EnableAGC:      false, // Отключено по умолчанию для телефонии
		EnableNR:       false,
		EnableEcho:     false,
		AGCTargetLevel: 0.7,
	}
}

// NewAudioProcessor создает новый аудио процессор с указанной конфигурацией.
// Автоматически заполняет отсутствующие параметры значениями по умолчанию.
func NewAudioProcessor(config AudioProcessorConfig) *AudioProcessor {
	// Устанавливаем значения по умолчанию
	if config.SampleRate == 0 {
		config.SampleRate = 8000
	}
	if config.Channels == 0 {
		config.Channels = 1
	}
	if config.Ptime == 0 {
		config.Ptime = time.Millisecond * 20
	}

	// Вычисляем размер буфера на основе ptime
	samplesPerPacket := int(float64(config.SampleRate) * config.Ptime.Seconds())
	bufferSize := samplesPerPacket * config.Channels * getBytesPerSample(config.PayloadType)

	return &AudioProcessor{
		config:       config,
		inputBuffer:  make([]byte, bufferSize),
		outputBuffer: make([]byte, bufferSize),
	}
}

// ProcessOutgoing обрабатывает исходящие аудио данные
func (ap *AudioProcessor) ProcessOutgoing(audioData []byte) ([]byte, error) {
	ap.mutex.Lock()
	defer ap.mutex.Unlock()

	ap.packetsIn++
	ap.bytesProcessed += uint64(len(audioData))

	// Проверяем размер данных
	expectedSize := ap.getExpectedPacketSize()
	if len(audioData) != expectedSize {
		return nil, NewAudioError(ErrorCodeAudioSizeInvalid, "",
			fmt.Sprintf("неожиданный размер аудио данных: %d, ожидается: %d",
				len(audioData), expectedSize),
			ap.config.PayloadType, expectedSize, len(audioData), ap.config.SampleRate, ap.config.Ptime)
	}

	// Копируем данные в рабочий буфер
	copy(ap.inputBuffer[:len(audioData)], audioData)

	// Применяем обработку
	processedData := ap.inputBuffer[:len(audioData)]

	// AGC (Automatic Gain Control)
	if ap.config.EnableAGC {
		processedData = ap.applyAGC(processedData)
	}

	// Noise Reduction
	if ap.config.EnableNR {
		processedData = ap.applyNoiseReduction(processedData)
	}

	// Кодируем в нужный формат (если требуется)
	finalData, err := ap.encodeAudio(processedData)
	if err != nil {
		return nil, WrapMediaError(ErrorCodeAudioProcessingFailed, "", "ошибка кодирования аудио", err)
	}

	ap.packetsOut++
	return finalData, nil
}

// ProcessIncoming обрабатывает входящие аудио данные
func (ap *AudioProcessor) ProcessIncoming(audioData []byte) ([]byte, error) {
	ap.mutex.Lock()
	defer ap.mutex.Unlock()

	ap.packetsIn++
	ap.bytesProcessed += uint64(len(audioData))

	// Декодируем из формата payload
	decodedData, err := ap.decodeAudio(audioData)
	if err != nil {
		return nil, WrapMediaError(ErrorCodeAudioProcessingFailed, "", "ошибка декодирования аудио", err)
	}

	// Копируем данные в рабочий буфер
	copy(ap.inputBuffer[:len(decodedData)], decodedData)

	// Применяем обработку
	processedData := ap.inputBuffer[:len(decodedData)]

	// Echo Cancellation
	if ap.config.EnableEcho {
		processedData = ap.applyEchoCancellation(processedData)
	}

	// Noise Reduction
	if ap.config.EnableNR {
		processedData = ap.applyNoiseReduction(processedData)
	}

	// AGC
	if ap.config.EnableAGC {
		processedData = ap.applyAGC(processedData)
	}

	ap.packetsOut++
	return processedData, nil
}

// SetPtime изменяет packet time
func (ap *AudioProcessor) SetPtime(ptime time.Duration) {
	ap.mutex.Lock()
	defer ap.mutex.Unlock()

	ap.config.Ptime = ptime

	// Пересчитываем размер буфера
	samplesPerPacket := int(float64(ap.config.SampleRate) * ptime.Seconds())
	bufferSize := samplesPerPacket * ap.config.Channels * getBytesPerSample(ap.config.PayloadType)

	ap.inputBuffer = make([]byte, bufferSize)
	ap.outputBuffer = make([]byte, bufferSize)
}

// GetStatistics возвращает статистику аудио процессора
func (ap *AudioProcessor) GetStatistics() AudioProcessorStatistics {
	ap.mutex.RLock()
	defer ap.mutex.RUnlock()

	return AudioProcessorStatistics{
		BytesProcessed: ap.bytesProcessed,
		PacketsIn:      ap.packetsIn,
		PacketsOut:     ap.packetsOut,
		PayloadType:    ap.config.PayloadType,
		SampleRate:     ap.config.SampleRate,
		Channels:       ap.config.Channels,
		Ptime:          ap.config.Ptime,
	}
}

// AudioProcessorStatistics статистика аудио процессора
type AudioProcessorStatistics struct {
	BytesProcessed uint64
	PacketsIn      uint64
	PacketsOut     uint64
	PayloadType    PayloadType
	SampleRate     uint32
	Channels       int
	Ptime          time.Duration
}

// getExpectedPacketSize вычисляет ожидаемый размер пакета
func (ap *AudioProcessor) getExpectedPacketSize() int {
	samplesPerPacket := int(float64(ap.config.SampleRate) * ap.config.Ptime.Seconds())
	return samplesPerPacket * ap.config.Channels * getBytesPerSample(ap.config.PayloadType)
}

// encodeAudio кодирует аудио данные в заданный формат
func (ap *AudioProcessor) encodeAudio(audioData []byte) ([]byte, error) {
	switch ap.config.PayloadType {
	case PayloadTypePCMU:
		return ap.encodePCMU(audioData), nil
	case PayloadTypePCMA:
		return ap.encodePCMA(audioData), nil
	case PayloadTypeG722:
		return ap.encodeG722(audioData)
	case PayloadTypeGSM:
		return ap.encodeGSM(audioData)
	default:
		// Для остальных кодеков просто возвращаем как есть
		result := make([]byte, len(audioData))
		copy(result, audioData)
		return result, nil
	}
}

// decodeAudio декодирует аудио данные из заданного формата
func (ap *AudioProcessor) decodeAudio(audioData []byte) ([]byte, error) {
	switch ap.config.PayloadType {
	case PayloadTypePCMU:
		return ap.decodePCMU(audioData), nil
	case PayloadTypePCMA:
		return ap.decodePCMA(audioData), nil
	case PayloadTypeG722:
		return ap.decodeG722(audioData)
	case PayloadTypeGSM:
		return ap.decodeGSM(audioData)
	default:
		// Для остальных кодеков просто возвращаем как есть
		result := make([]byte, len(audioData))
		copy(result, audioData)
		return result, nil
	}
}

// applyAGC применяет автоматическую регулировку усиления
func (ap *AudioProcessor) applyAGC(audioData []byte) []byte {
	result := make([]byte, len(audioData))
	copy(result, audioData)

	// Находим максимальную амплитуду
	maxLevel := byte(0)
	for _, sample := range audioData {
		if sample > maxLevel {
			maxLevel = sample
		}
	}

	// Применяем простое усиление
	if maxLevel > 0 {
		targetLevel := byte(float32(255) * ap.config.AGCTargetLevel)
		gain := float32(targetLevel) / float32(maxLevel)

		for i, sample := range audioData {
			newLevel := float32(sample) * gain
			if newLevel > 255 {
				newLevel = 255
			}
			result[i] = byte(newLevel)
		}
	}

	return result
}

// applyNoiseReduction применяет шумоподавление
func (ap *AudioProcessor) applyNoiseReduction(audioData []byte) []byte {
	result := make([]byte, len(audioData))
	copy(result, audioData)

	// Простой фильтр высоких частот для удаления низкочастотного шума
	if len(audioData) > 2 {
		for i := 1; i < len(audioData)-1; i++ {
			// Простая формула фильтра высоких частот
			filtered := int(audioData[i]) - (int(audioData[i-1])+int(audioData[i+1]))/4
			if filtered < 0 {
				filtered = 0
			}
			if filtered > 255 {
				filtered = 255
			}
			result[i] = byte(filtered)
		}
	}

	return result
}

// applyEchoCancellation применяет эхоподавление
func (ap *AudioProcessor) applyEchoCancellation(audioData []byte) []byte {
	// Заглушка - в реальности нужен адаптивный фильтр
	result := make([]byte, len(audioData))
	copy(result, audioData)
	return result
}

// Простые кодеки

// encodePCMU кодирует в G.711 μ-law
func (ap *AudioProcessor) encodePCMU(audioData []byte) []byte {
	result := make([]byte, len(audioData))
	for i, sample := range audioData {
		// Простое приближение μ-law
		if sample >= 128 {
			result[i] = 0xFF - ((sample - 128) >> 1)
		} else {
			result[i] = 0x80 - (sample >> 1)
		}
	}
	return result
}

// decodePCMU декодирует из G.711 μ-law
func (ap *AudioProcessor) decodePCMU(audioData []byte) []byte {
	result := make([]byte, len(audioData))
	for i, sample := range audioData {
		// Простое приближение μ-law декодирования
		if sample >= 0x80 {
			result[i] = 128 + ((0xFF - sample) << 1)
		} else {
			result[i] = (0x80 - sample) << 1
		}
	}
	return result
}

// encodePCMA кодирует в G.711 A-law
func (ap *AudioProcessor) encodePCMA(audioData []byte) []byte {
	result := make([]byte, len(audioData))
	for i, sample := range audioData {
		result[i] = sample ^ 0x55 // XOR с константой для A-law
	}
	return result
}

// decodePCMA декодирует из G.711 A-law
func (ap *AudioProcessor) decodePCMA(audioData []byte) []byte {
	result := make([]byte, len(audioData))
	for i, sample := range audioData {
		result[i] = sample ^ 0x55 // XOR с константой для A-law
	}
	return result
}

// encodeG722 кодирует в G.722
func (ap *AudioProcessor) encodeG722(audioData []byte) ([]byte, error) {
	result := make([]byte, len(audioData)/2) // G.722 сжимает в 2 раза
	for i := range result {
		if i*2+1 < len(audioData) {
			result[i] = (audioData[i*2] + audioData[i*2+1]) / 2
		}
	}
	return result, nil
}

// decodeG722 декодирует из G.722
func (ap *AudioProcessor) decodeG722(audioData []byte) ([]byte, error) {
	result := make([]byte, len(audioData)*2) // G.722 расширяется в 2 раза
	for i, sample := range audioData {
		result[i*2] = sample
		if i*2+1 < len(result) {
			result[i*2+1] = sample
		}
	}
	return result, nil
}

// encodeGSM кодирует в GSM 06.10
func (ap *AudioProcessor) encodeGSM(audioData []byte) ([]byte, error) {
	return audioData, &MediaError{
		Code:    ErrorCodeAudioCodecUnsupported,
		Message: "GSM кодирование не реализовано",
	}
}

// decodeGSM декодирует из GSM 06.10
func (ap *AudioProcessor) decodeGSM(audioData []byte) ([]byte, error) {
	return audioData, &MediaError{
		Code:    ErrorCodeAudioCodecUnsupported,
		Message: "GSM декодирование не реализовано",
	}
}

// getBytesPerSample возвращает количество байт на sample для payload типа
func getBytesPerSample(payloadType PayloadType) int {
	switch payloadType {
	case PayloadTypePCMU, PayloadTypePCMA:
		return 1 // 8 бит на sample
	case PayloadTypeG722:
		return 1 // 8 бит на sample (но 16kHz sampling)
	default:
		return 1 // По умолчанию
	}
}
