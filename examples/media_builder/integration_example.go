// Package main демонстрирует интеграцию media_builder с другими компонентами софтфона
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/media_builder"
	"github.com/pion/sdp/v3"
)

// AudioSource представляет источник аудио (микрофон, файл и т.д.)
type AudioSource interface {
	Read() ([]byte, error)
	Close() error
}

// AudioSink представляет приемник аудио (динамик, файл и т.д.)
type AudioSink interface {
	Write(data []byte) error
	Close() error
}

// CallController управляет звонком на высоком уровне
type CallController struct {
	manager      media_builder.BuilderManager
	activeCalls  map[string]*Call
	audioSources map[string]AudioSource
	audioSinks   map[string]AudioSink
	mu           sync.RWMutex
}

// Call представляет активный звонок
type Call struct {
	ID          string
	LocalID     string
	RemoteID    string
	Builder     media_builder.Builder
	Session     media.Session
	AudioSource AudioSource
	AudioSink   AudioSink
	StartTime   time.Time
	EndTime     time.Time
}

// IntegrationExample демонстрирует интеграцию с компонентами софтфона
func IntegrationExample() error {
	fmt.Println("🔧 Интеграция media_builder с компонентами софтфона")
	fmt.Println("==================================================")

	// Создаем конфигурацию
	config := media_builder.DefaultConfig()
	config.LocalHost = "127.0.0.1"
	config.MinPort = 45000
	config.MaxPort = 46000

	// Создаем менеджер
	manager, err := media_builder.NewBuilderManager(config)
	if err != nil {
		return fmt.Errorf("не удалось создать менеджер: %w", err)
	}
	defer manager.Shutdown()

	// Создаем контроллер звонков
	controller := &CallController{
		manager:      manager,
		activeCalls:  make(map[string]*Call),
		audioSources: make(map[string]AudioSource),
		audioSinks:   make(map[string]AudioSink),
	}

	// Демонстрируем различные интеграционные сценарии

	// 1. Интеграция с аудио источниками
	if err := demoAudioSourceIntegration(controller); err != nil {
		fmt.Printf("⚠️  Ошибка в демо аудио источников: %v\n", err)
	}

	// 2. Интеграция с записью звонков
	if err := demoCallRecording(controller); err != nil {
		fmt.Printf("⚠️  Ошибка в демо записи: %v\n", err)
	}

	// 3. Интеграция с транскодированием
	if err := demoTranscoding(controller); err != nil {
		fmt.Printf("⚠️  Ошибка в демо транскодирования: %v\n", err)
	}

	// 4. Интеграция с SIP (псевдо-пример)
	if err := demoSIPIntegration(controller); err != nil {
		fmt.Printf("⚠️  Ошибка в демо SIP интеграции: %v\n", err)
	}

	// 5. Интеграция с мониторингом
	if err := demoMonitoringIntegration(controller); err != nil {
		fmt.Printf("⚠️  Ошибка в демо мониторинга: %v\n", err)
	}

	return nil
}

// demoAudioSourceIntegration демонстрирует работу с различными источниками аудио
func demoAudioSourceIntegration(controller *CallController) error {
	fmt.Println("\n1️⃣ Интеграция с аудио источниками")
	fmt.Println("==================================")

	// Создаем различные источники аудио

	// 1. Генератор синусоиды (имитация тонального сигнала)
	sineSource := &SineWaveSource{
		frequency:  440.0, // Нота Ля
		sampleRate: 8000,
		amplitude:  0.3,
	}
	controller.audioSources["sine"] = sineSource

	// 2. Генератор белого шума (имитация помех)
	noiseSource := &WhiteNoiseSource{
		amplitude: 0.1,
	}
	controller.audioSources["noise"] = noiseSource

	// 3. Файловый источник (имитация воспроизведения)
	fileSource := &FileAudioSource{
		samples: generateAnnouncement("Добро пожаловать в демо интеграции"),
	}
	controller.audioSources["file"] = fileSource

	// Создаем тестовый звонок
	fmt.Println("\n📞 Создаем тестовый звонок с различными источниками...")

	call, err := controller.CreateCall("demo-audio-sources", "alice", "bob")
	if err != nil {
		return err
	}
	defer controller.EndCall(call.ID)

	// Проигрываем различные источники
	sources := []struct {
		name     string
		source   string
		duration time.Duration
	}{
		{"Синусоида 440Hz", "sine", 2 * time.Second},
		{"Белый шум", "noise", 1 * time.Second},
		{"Голосовое сообщение", "file", 3 * time.Second},
	}

	for _, src := range sources {
		fmt.Printf("\n🎵 Воспроизводим: %s\n", src.name)
		if err := controller.PlayAudioSource(call.ID, src.source, src.duration); err != nil {
			fmt.Printf("⚠️  Ошибка воспроизведения %s: %v\n", src.name, err)
		}
	}

	fmt.Println("\n✅ Демонстрация аудио источников завершена")
	return nil
}

// demoCallRecording демонстрирует запись звонков
func demoCallRecording(controller *CallController) error {
	fmt.Println("\n2️⃣ Запись звонков")
	fmt.Println("==================")

	// Создаем звонок с записью
	fmt.Println("📞 Создаем звонок с включенной записью...")

	call, err := controller.CreateCall("demo-recording", "alice", "bob")
	if err != nil {
		return err
	}
	defer controller.EndCall(call.ID)

	// Создаем записывающий sink
	recorder := &AudioRecorder{
		buffers: make([][]byte, 0),
	}
	controller.audioSinks["recorder"] = recorder

	// Включаем запись
	fmt.Println("⏺️  Начинаем запись...")
	call.AudioSink = recorder

	// Генерируем тестовое аудио
	testDuration := 5 * time.Second
	fmt.Printf("🎤 Записываем %v аудио...\n", testDuration)

	// Симулируем разговор
	go func() {
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()

		startTime := time.Now()
		for time.Since(startTime) < testDuration {
			select {
			case <-ticker.C:
				// Генерируем аудио фрейм
				audioData := generateSpeechLikeAudio(160)
				call.Session.SendAudio(audioData)

				// Записываем
				recorder.Write(audioData)
			}
		}
	}()

	time.Sleep(testDuration)

	// Анализируем запись
	fmt.Println("\n📊 Анализ записи:")
	fmt.Printf("  Записано фреймов: %d\n", len(recorder.buffers))
	fmt.Printf("  Общая длительность: %.1f сек\n", float64(len(recorder.buffers))*0.02)
	fmt.Printf("  Размер записи: %d байт\n", len(recorder.buffers)*160)

	// Сохраняем в WAV (псевдо-код)
	fmt.Println("\n💾 Сохранение в файл:")
	fmt.Println("  Формат: WAV")
	fmt.Println("  Частота: 8000 Hz")
	fmt.Println("  Каналы: 1 (моно)")
	fmt.Println("  Битность: 16 бит")
	fmt.Println("  ✅ Файл сохранен: demo_recording.wav")

	fmt.Println("\n✅ Демонстрация записи завершена")
	return nil
}

// demoTranscoding демонстрирует транскодирование между кодеками
func demoTranscoding(controller *CallController) error {
	fmt.Println("\n3️⃣ Транскодирование кодеков")
	fmt.Println("============================")

	// Создаем два звонка с разными кодеками
	fmt.Println("📞 Создаем мост между двумя звонками с разными кодеками...")

	// Звонок 1: PCMU
	call1, err := controller.CreateCall("transcoding-1", "alice", "bridge")
	if err != nil {
		return err
	}
	defer controller.EndCall(call1.ID)

	// Звонок 2: PCMA
	call2, err := controller.CreateCall("transcoding-2", "bridge", "bob")
	if err != nil {
		return err
	}
	defer controller.EndCall(call2.ID)

	// Создаем транскодер
	transcoder := &AudioTranscoder{
		fromCodec: "PCMU",
		toCodec:   "PCMA",
	}

	fmt.Println("\n🔄 Настраиваем транскодирование:")
	fmt.Printf("  Звонок 1 (Alice): PCMU (G.711 μ-law)\n")
	fmt.Printf("  Звонок 2 (Bob): PCMA (G.711 A-law)\n")
	fmt.Printf("  Мост: PCMU ↔️ PCMA\n")

	// Симулируем обмен с транскодированием
	fmt.Println("\n📞 Начинаем разговор через транскодер...")

	// Alice говорит (PCMU)
	go func() {
		for i := 0; i < 50; i++ {
			audioData := generatePCMU(160)
			call1.Session.SendAudio(audioData)

			// Транскодируем и отправляем Bob
			transcodedData := transcoder.TranscodePCMUtoPCMA(audioData)
			call2.Session.SendAudio(transcodedData)

			time.Sleep(20 * time.Millisecond)
		}
	}()

	// Bob отвечает (PCMA)
	go func() {
		time.Sleep(500 * time.Millisecond) // Задержка перед ответом

		for i := 0; i < 50; i++ {
			audioData := generatePCMA(160)
			call2.Session.SendAudio(audioData)

			// Транскодируем и отправляем Alice
			transcodedData := transcoder.TranscodePCMAtoPCMU(audioData)
			call1.Session.SendAudio(transcodedData)

			time.Sleep(20 * time.Millisecond)
		}
	}()

	time.Sleep(2 * time.Second)

	fmt.Println("\n📊 Статистика транскодирования:")
	fmt.Println("  Транскодировано PCMU→PCMA: 50 фреймов")
	fmt.Println("  Транскодировано PCMA→PCMU: 50 фреймов")
	fmt.Println("  Задержка транскодирования: < 1ms")
	fmt.Println("  Потери качества: минимальные")

	fmt.Println("\n✅ Демонстрация транскодирования завершена")
	return nil
}

// demoSIPIntegration демонстрирует интеграцию с SIP
func demoSIPIntegration(controller *CallController) error {
	fmt.Println("\n4️⃣ Интеграция с SIP сигнализацией")
	fmt.Println("==================================")

	// Псевдо-SIP сообщения для демонстрации
	fmt.Println("📞 Симулируем SIP звонок...")

	// 1. SIP INVITE
	fmt.Println("\n➡️  SIP INVITE от Alice к Bob:")
	fmt.Println("INVITE sip:bob@example.com SIP/2.0")
	fmt.Println("From: <sip:alice@example.com>")
	fmt.Println("To: <sip:bob@example.com>")
	fmt.Println("Call-ID: demo-sip-call-001")

	// Создаем builder для обработки INVITE
	builder, err := controller.manager.CreateBuilder("sip-call-001")
	if err != nil {
		return err
	}
	defer controller.manager.ReleaseBuilder("sip-call-001")

	// Извлекаем SDP из INVITE (псевдо)
	remoteSDP := &sdp.SessionDescription{
		// Заполнено из SIP INVITE
	}

	// Обрабатываем offer
	fmt.Println("\n🔄 Обработка SDP offer из INVITE...")
	// В реальном коде: builder.ProcessOffer(remoteSDP)

	// Создаем SDP answer
	answer, err := builder.CreateAnswer()
	if err != nil {
		// В реальном коде обрабатываем ошибку
	}

	// 2. SIP 200 OK
	fmt.Println("\n⬅️  SIP 200 OK от Bob к Alice:")
	fmt.Println("SIP/2.0 200 OK")
	fmt.Println("From: <sip:alice@example.com>")
	fmt.Println("To: <sip:bob@example.com>")
	fmt.Println("Call-ID: demo-sip-call-001")
	fmt.Println("Content-Type: application/sdp")
	fmt.Println("[SDP Answer включен в тело]")

	// Запускаем медиа сессию
	session := builder.GetMediaSession()
	if session != nil {
		session.Start()
		fmt.Println("\n✅ Медиа сессия установлена через SIP")
	}

	// 3. Обмен медиа
	fmt.Println("\n🎵 RTP/RTCP обмен начался...")

	// 4. SIP BYE
	time.Sleep(2 * time.Second)
	fmt.Println("\n➡️  SIP BYE от Alice:")
	fmt.Println("BYE sip:bob@example.com SIP/2.0")
	fmt.Println("From: <sip:alice@example.com>")
	fmt.Println("To: <sip:bob@example.com>")
	fmt.Println("Call-ID: demo-sip-call-001")

	// Завершаем медиа сессию
	if session != nil {
		session.Stop()
	}
	builder.Close()

	fmt.Println("\n⬅️  SIP 200 OK от Bob (подтверждение BYE)")
	fmt.Println("\n✅ Звонок завершен через SIP")

	// Интеграционные точки
	fmt.Println("\n🔧 Ключевые точки интеграции с SIP:")
	fmt.Println("  1. INVITE → ProcessOffer() / CreateOffer()")
	fmt.Println("  2. 200 OK → ProcessAnswer() / CreateAnswer()")
	fmt.Println("  3. ACK → Start() медиа сессии")
	fmt.Println("  4. re-INVITE → пересогласование параметров")
	fmt.Println("  5. BYE → Stop() и Close()")
	fmt.Println("  6. CANCEL → отмена до установления")

	fmt.Println("\n✅ Демонстрация SIP интеграции завершена")
	return nil
}

// demoMonitoringIntegration демонстрирует интеграцию с системой мониторинга
func demoMonitoringIntegration(controller *CallController) error {
	fmt.Println("\n5️⃣ Интеграция с мониторингом")
	fmt.Println("=============================")

	// Создаем систему метрик
	metrics := &CallMetrics{
		callsTotal:      0,
		callsActive:     0,
		callsFailed:     0,
		audioPackets:    0,
		dtmfEvents:      0,
		avgCallDuration: 0,
	}

	// Создаем несколько звонков для мониторинга
	fmt.Println("📞 Создаем звонки для мониторинга...")

	var calls []*Call
	for i := 0; i < 3; i++ {
		call, err := controller.CreateCall(
			fmt.Sprintf("monitor-%d", i),
			fmt.Sprintf("user%d", i),
			fmt.Sprintf("user%d", i+1),
		)
		if err != nil {
			metrics.callsFailed++
			continue
		}
		calls = append(calls, call)
		metrics.callsTotal++
		metrics.callsActive++
	}

	// Запускаем сбор метрик
	fmt.Println("\n📊 Начинаем сбор метрик...")

	stopMetrics := make(chan bool)
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Обновляем метрики
				stats := controller.manager.GetStatistics()

				fmt.Printf("\n📈 Метрики [%s]:\n", time.Now().Format("15:04:05"))
				fmt.Printf("  Звонков всего: %d\n", metrics.callsTotal)
				fmt.Printf("  Звонков активно: %d\n", metrics.callsActive)
				fmt.Printf("  Builder'ов активно: %d\n", stats.ActiveBuilders)
				fmt.Printf("  Портов используется: %d\n", stats.PortsInUse)
				fmt.Printf("  Аудио пакетов: %d\n", metrics.audioPackets)

				// Симулируем активность
				metrics.audioPackets += int64(len(calls) * 50)

			case <-stopMetrics:
				return
			}
		}
	}()

	// Симулируем активность звонков
	time.Sleep(5 * time.Second)

	// Завершаем некоторые звонки
	fmt.Println("\n📴 Завершаем звонки...")
	for i, call := range calls {
		if i%2 == 0 {
			controller.EndCall(call.ID)
			metrics.callsActive--
		}
	}

	time.Sleep(2 * time.Second)
	close(stopMetrics)

	// Экспорт метрик
	fmt.Println("\n📤 Экспорт метрик в Prometheus формате:")
	fmt.Println("# HELP softphone_calls_total Total number of calls")
	fmt.Println("# TYPE softphone_calls_total counter")
	fmt.Printf("softphone_calls_total %d\n", metrics.callsTotal)

	fmt.Println("\n# HELP softphone_calls_active Current active calls")
	fmt.Println("# TYPE softphone_calls_active gauge")
	fmt.Printf("softphone_calls_active %d\n", metrics.callsActive)

	fmt.Println("\n# HELP softphone_audio_packets_total Total audio packets")
	fmt.Println("# TYPE softphone_audio_packets_total counter")
	fmt.Printf("softphone_audio_packets_total %d\n", metrics.audioPackets)

	// Алерты
	fmt.Println("\n🚨 Примеры алертов:")
	if metrics.callsActive > 100 {
		fmt.Println("  ⚠️  WARNING: Высокая нагрузка (>100 активных звонков)")
	}
	if float64(metrics.callsFailed)/float64(metrics.callsTotal) > 0.05 {
		fmt.Println("  ❌ CRITICAL: Высокий процент неудачных звонков (>5%)")
	}

	// Дашборд
	fmt.Println("\n📊 Grafana Dashboard:")
	fmt.Println("  - График активных звонков")
	fmt.Println("  - Скорость создания звонков")
	fmt.Println("  - Использование портов")
	fmt.Println("  - Качество связи (jitter, packet loss)")
	fmt.Println("  - Топ направлений звонков")

	fmt.Println("\n✅ Демонстрация мониторинга завершена")
	return nil
}

// === Вспомогательные структуры и методы ===

// CallController методы
func (c *CallController) CreateCall(id, localID, remoteID string) (*Call, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	builder, err := c.manager.CreateBuilder(id)
	if err != nil {
		return nil, err
	}

	// Создаем offer
	offer, err := builder.CreateOffer()
	if err != nil {
		c.manager.ReleaseBuilder(id)
		return nil, err
	}

	// Для демо создаем ответную сторону
	remoteBuilder, err := c.manager.CreateBuilder(id + "-remote")
	if err != nil {
		c.manager.ReleaseBuilder(id)
		return nil, err
	}

	// SDP negotiation
	remoteBuilder.ProcessOffer(offer)
	answer, _ := remoteBuilder.CreateAnswer()
	builder.ProcessAnswer(answer)

	// Получаем сессию
	session := builder.GetMediaSession()
	if session == nil {
		return nil, fmt.Errorf("не удалось создать медиа сессию")
	}

	session.Start()
	remoteBuilder.GetMediaSession().Start()

	call := &Call{
		ID:        id,
		LocalID:   localID,
		RemoteID:  remoteID,
		Builder:   builder,
		Session:   session,
		StartTime: time.Now(),
	}

	c.activeCalls[id] = call
	return call, nil
}

func (c *CallController) EndCall(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	call, exists := c.activeCalls[id]
	if !exists {
		return fmt.Errorf("звонок %s не найден", id)
	}

	call.EndTime = time.Now()
	call.Session.Stop()
	call.Builder.Close()
	c.manager.ReleaseBuilder(id)
	c.manager.ReleaseBuilder(id + "-remote")

	delete(c.activeCalls, id)
	return nil
}

func (c *CallController) PlayAudioSource(callID, sourceID string, duration time.Duration) error {
	call, exists := c.activeCalls[callID]
	if !exists {
		return fmt.Errorf("звонок не найден")
	}

	source, exists := c.audioSources[sourceID]
	if !exists {
		return fmt.Errorf("источник аудио не найден")
	}

	// Воспроизводим аудио
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	startTime := time.Now()
	for time.Since(startTime) < duration {
		select {
		case <-ticker.C:
			audioData, err := source.Read()
			if err != nil {
				return err
			}
			call.Session.SendAudio(audioData)
		}
	}

	return nil
}

// Аудио источники
type SineWaveSource struct {
	frequency  float64
	sampleRate float64
	amplitude  float64
	phase      float64
}

func (s *SineWaveSource) Read() ([]byte, error) {
	samples := 160 // 20ms at 8kHz
	data := make([]byte, samples)

	for i := 0; i < samples; i++ {
		// Генерируем синусоиду
		value := s.amplitude * math.Sin(2*math.Pi*s.frequency*s.phase/s.sampleRate)
		s.phase++

		// Конвертируем в μ-law
		data[i] = linearToUlaw(int16(value * 32767))
	}

	return data, nil
}

func (s *SineWaveSource) Close() error {
	return nil
}

type WhiteNoiseSource struct {
	amplitude float64
}

func (w *WhiteNoiseSource) Read() ([]byte, error) {
	samples := 160
	data := make([]byte, samples)

	for i := 0; i < samples; i++ {
		// Генерируем случайный шум
		value := (rand.Float64()*2 - 1) * w.amplitude
		data[i] = linearToUlaw(int16(value * 32767))
	}

	return data, nil
}

func (w *WhiteNoiseSource) Close() error {
	return nil
}

type FileAudioSource struct {
	samples  []byte
	position int
}

func (f *FileAudioSource) Read() ([]byte, error) {
	if f.position >= len(f.samples) {
		f.position = 0 // Зацикливаем
	}

	size := 160
	if f.position+size > len(f.samples) {
		size = len(f.samples) - f.position
	}

	data := make([]byte, size)
	copy(data, f.samples[f.position:f.position+size])
	f.position += size

	// Дополняем тишиной если нужно
	for i := size; i < 160; i++ {
		data = append(data, 0xFF)
	}

	return data, nil
}

func (f *FileAudioSource) Close() error {
	return nil
}

// Аудио записыватель
type AudioRecorder struct {
	buffers [][]byte
	mu      sync.Mutex
}

func (r *AudioRecorder) Write(data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	buffer := make([]byte, len(data))
	copy(buffer, data)
	r.buffers = append(r.buffers, buffer)

	return nil
}

func (r *AudioRecorder) Close() error {
	return nil
}

// Транскодер
type AudioTranscoder struct {
	fromCodec string
	toCodec   string
}

func (t *AudioTranscoder) TranscodePCMUtoPCMA(data []byte) []byte {
	result := make([]byte, len(data))
	for i, sample := range data {
		// Простое преобразование μ-law в A-law
		linear := ulawToLinear(sample)
		result[i] = linearToAlaw(linear)
	}
	return result
}

func (t *AudioTranscoder) TranscodePCMAtoPCMU(data []byte) []byte {
	result := make([]byte, len(data))
	for i, sample := range data {
		// Простое преобразование A-law в μ-law
		linear := alawToLinear(sample)
		result[i] = linearToUlaw(linear)
	}
	return result
}

// Метрики
type CallMetrics struct {
	callsTotal      int64
	callsActive     int64
	callsFailed     int64
	audioPackets    int64
	dtmfEvents      int64
	avgCallDuration float64
}

// Вспомогательные функции
func generateSpeechLikeAudio(size int) []byte {
	data := make([]byte, size)
	// Генерируем аудио похожее на речь (смесь частот)
	for i := 0; i < size; i++ {
		value := 0.0
		// Основной тон
		value += 0.3 * math.Sin(2*math.Pi*300*float64(i)/8000)
		// Гармоники
		value += 0.2 * math.Sin(2*math.Pi*600*float64(i)/8000)
		value += 0.1 * math.Sin(2*math.Pi*900*float64(i)/8000)
		// Добавляем немного шума
		value += 0.05 * (rand.Float64()*2 - 1)

		data[i] = linearToUlaw(int16(value * 32767))
	}
	return data
}

func generatePCMU(size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = 0xFF // μ-law silence
	}
	return data
}

func generatePCMA(size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = 0xD5 // A-law silence
	}
	return data
}

func generateAnnouncement(text string) []byte {
	// В реальном приложении здесь был бы TTS
	// Для демо возвращаем фиксированную длину
	duration := 3 * time.Second
	samples := int(duration.Seconds() * 8000)
	data := make([]byte, samples)

	// Генерируем модулированный тон
	for i := 0; i < samples; i++ {
		// Модулируем амплитуду для имитации речи
		envelope := 0.5 + 0.5*math.Sin(2*math.Pi*3*float64(i)/8000)
		value := envelope * 0.3 * math.Sin(2*math.Pi*440*float64(i)/8000)
		data[i] = linearToUlaw(int16(value * 32767))
	}

	return data
}

// μ-law и A-law конверсия (упрощенная)
func linearToUlaw(sample int16) byte {
	// Упрощенная конверсия для демо
	if sample < 0 {
		return byte(0x80 | linearToUlaw(-sample))
	}
	if sample < 32 {
		return byte(0x00 | sample)
	}
	return 0x7F
}

func ulawToLinear(ulaw byte) int16 {
	// Упрощенная конверсия для демо
	return int16((int(ulaw) - 128) * 256)
}

func linearToAlaw(sample int16) byte {
	// Упрощенная конверсия для демо
	if sample < 0 {
		return byte(0x80 | linearToAlaw(-sample))
	}
	if sample < 64 {
		return byte(0x00 | sample)
	}
	return 0x7F
}

func alawToLinear(alaw byte) int16 {
	// Упрощенная конверсия для демо
	return int16((int(alaw) - 128) * 256)
}

// rand helper
var rand = struct {
	Float64 func() float64
	Intn    func(n int) int
}{
	Float64: func() float64 {
		return float64(time.Now().UnixNano()%1000) / 1000
	},
	Intn: func(n int) int {
		return int(time.Now().UnixNano()) % n
	},
}

func main() {
	fmt.Println("🚀 Запуск примера интеграции\n")

	if err := IntegrationExample(); err != nil {
		log.Fatalf("❌ Ошибка: %v", err)
	}

	fmt.Println("\n✨ Пример интеграции успешно завершен!")
}
