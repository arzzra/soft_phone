// race_test.go - тесты для проверки отсутствия race conditions
package rtp

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/pion/rtp"
)

// TestConcurrentHandlerRegistration тестирует concurrent регистрацию обработчиков
// Проверяет отсутствие race conditions при одновременной установке/вызове handlers
func TestConcurrentHandlerRegistration(t *testing.T) {
	transport := NewMockTransport()
	transport.SetActive(true)

	config := RTPSessionConfig{
		SSRC:        0x12345678,
		PayloadType: PayloadTypePCMU,
		ClockRate:   8000,
		Transport:   transport,
	}

	session, err := NewRTPSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer session.Stop()

	err = session.Start()
	if err != nil {
		t.Fatalf("Ошибка запуска сессии: %v", err)
	}

	var wg sync.WaitGroup

	// Запускаем горутины для concurrent регистрации обработчиков
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(handlerID int) {
			defer wg.Done()

			// Регистрируем обработчик входящих пакетов
			session.RegisterIncomingHandler(func(packet *rtp.Packet, addr net.Addr) {
				// Симулируем обработку
				time.Sleep(time.Microsecond * 100)
			})

			// Регистрируем обработчик отправленных пакетов
			session.RegisterSentHandler(func(packet *rtp.Packet) {
				// Симулируем обработку
				time.Sleep(time.Microsecond * 100)
			})
		}(i)
	}

	// Параллельно отправляем пакеты
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			audioData := generateTestAudioData(160)
			err := session.SendAudio(audioData, time.Millisecond*20)
			if err != nil {
				t.Errorf("Ошибка отправки аудио: %v", err)
			}
		}()
	}

	// Параллельно симулируем получение пакетов
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(packetID int) {
			defer wg.Done()
			
			packet := &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					PayloadType:    uint8(PayloadTypePCMU),
					SequenceNumber: uint16(packetID + 1000),
					Timestamp:      uint32(packetID * 160),
					SSRC:           0x87654321,
				},
				Payload: generateTestAudioData(160),
			}
			
			transport.SimulateReceive(packet)
		}(i)
	}

	// Ждем завершения всех горутин
	wg.Wait()

	// Ждем обработки пакетов
	time.Sleep(time.Millisecond * 50)

	t.Logf("Тест concurrent handler registration завершен успешно")
}

// TestHighFrequencyHandlerRegistration тестирует частую смену обработчиков
func TestHighFrequencyHandlerRegistration(t *testing.T) {
	transport := NewMockTransport()
	transport.SetActive(true)

	config := RTPSessionConfig{
		SSRC:        0x12345678,
		PayloadType: PayloadTypePCMU,
		ClockRate:   8000,
		Transport:   transport,
	}

	session, err := NewRTPSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer session.Stop()

	session.Start()

	var wg sync.WaitGroup
	done := make(chan struct{})

	// Постоянно меняем обработчики
	wg.Add(1)
	go func() {
		defer wg.Done()
		counter := 0
		for {
			select {
			case <-done:
				return
			default:
				counter++
				currentCounter := counter // Захватываем значение, а не ссылку
				session.RegisterIncomingHandler(func(packet *rtp.Packet, addr net.Addr) {
					// Обработчик версии currentCounter
					_ = currentCounter
				})
				time.Sleep(time.Microsecond * 10)
			}
		}
	}()

	// Постоянно отправляем пакеты
	wg.Add(1)
	go func() {
		defer wg.Done()
		audioData := generateTestAudioData(160)
		for {
			select {
			case <-done:
				return
			default:
				session.SendAudio(audioData, time.Millisecond*20)
				time.Sleep(time.Microsecond * 50)
			}
		}
	}()

	// Постоянно симулируем получение пакетов
	wg.Add(1)
	go func() {
		defer wg.Done()
		packetID := 0
		for {
			select {
			case <-done:
				return
			default:
				packetID++
				packet := &rtp.Packet{
					Header: rtp.Header{
						Version:        2,
						PayloadType:    uint8(PayloadTypePCMU),
						SequenceNumber: uint16(packetID),
						Timestamp:      uint32(packetID * 160),
						SSRC:           0x87654321,
					},
					Payload: generateTestAudioData(160),
				}
				transport.SimulateReceive(packet)
				time.Sleep(time.Microsecond * 30)
			}
		}
	}()

	// Тестируем в течение 100ms
	time.Sleep(time.Millisecond * 100)
	close(done)
	wg.Wait()

	t.Logf("Тест high frequency handler registration завершен успешно")
}

// TestConcurrentSessionOperations тестирует concurrent операции с сессией
func TestConcurrentSessionOperations(t *testing.T) {
	transport := NewMockTransport()
	transport.SetActive(true)

	config := RTPSessionConfig{
		SSRC:        0x12345678,
		PayloadType: PayloadTypePCMU,
		ClockRate:   8000,
		Transport:   transport,
	}

	session, err := NewRTPSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer session.Stop()

	session.Start()

	var wg sync.WaitGroup
	numGoroutines := 20

	// Concurrent чтение статистики
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = session.GetSSRC()
				_ = session.GetSequenceNumber()
				_ = session.GetTimestamp()
				_ = session.GetPacketsSent()
				_ = session.GetPacketsReceived()
				_ = session.IsActive()
			}
		}()
	}

	// Concurrent отправка пакетов
	for i := 0; i < numGoroutines/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			audioData := generateTestAudioData(160)
			for j := 0; j < 50; j++ {
				session.SendAudio(audioData, time.Millisecond*20)
				time.Sleep(time.Microsecond * 100)
			}
		}()
	}

	// Concurrent регистрация обработчиков
	for i := 0; i < numGoroutines/4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				session.RegisterIncomingHandler(func(packet *rtp.Packet, addr net.Addr) {
					// Тестовый обработчик
				})
				time.Sleep(time.Millisecond * 5)
			}
		}()
	}

	wg.Wait()
	t.Logf("Тест concurrent session operations завершен успешно")
}