// rate_limiting_test.go - тесты для rate limiting функциональности
package rtp

import (
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/rtp"
)

// TestSourceManagerRateLimiting тестирует rate limiting на уровне SourceManager
func TestSourceManagerRateLimiting(t *testing.T) {
	var rateLimitedCount int32
	var rateLimitedSSRC uint32

	config := SourceManagerConfig{
		MaxPacketsPerSecond: 5,                   // Максимум 5 пакетов в секунду
		RateLimitWindow:     time.Second,         // Окно 1 секунда
		OnRateLimited: func(ssrc uint32, source *RemoteSource) {
			atomic.AddInt32(&rateLimitedCount, 1)
			atomic.StoreUint32(&rateLimitedSSRC, ssrc)
		},
	}

	sm := NewSourceManager(config)
	defer sm.Stop()

	testSSRC := uint32(0x12345678)

	// Отправляем пакеты в пределах лимита
	for i := 0; i < 5; i++ {
		packet := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    0,
				SequenceNumber: uint16(i + 1),
				Timestamp:      uint32(i * 160),
				SSRC:           testSSRC,
			},
			Payload: make([]byte, 160),
		}

		source := sm.UpdateFromPacket(packet)
		if source == nil {
			t.Errorf("Пакет %d должен быть принят (в пределах лимита)", i+1)
		}
	}

	// Проверяем что rate limiting еще не сработал
	if atomic.LoadInt32(&rateLimitedCount) != 0 {
		t.Errorf("Rate limiting не должен сработать для %d пакетов", 5)
	}

	// Отправляем пакеты сверх лимита
	for i := 5; i < 10; i++ {
		packet := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    0,
				SequenceNumber: uint16(i + 1),
				Timestamp:      uint32(i * 160),
				SSRC:           testSSRC,
			},
			Payload: make([]byte, 160),
		}

		source := sm.UpdateFromPacket(packet)
		if source != nil {
			t.Errorf("Пакет %d должен быть заблокирован (превышение лимита)", i+1)
		}
	}

	// Ждем немного для обработки async вызовов
	time.Sleep(time.Millisecond * 10)

	// Проверяем что rate limiting сработал
	if atomic.LoadInt32(&rateLimitedCount) == 0 {
		t.Error("Rate limiting должен сработать при превышении лимита")
	}

	if atomic.LoadUint32(&rateLimitedSSRC) != testSSRC {
		t.Errorf("Rate limited SSRC неверный: получен %x, ожидался %x", 
			atomic.LoadUint32(&rateLimitedSSRC), testSSRC)
	}

	// Проверяем список заблокированных источников
	limited := sm.GetRateLimitedSources()
	if len(limited) != 1 {
		t.Errorf("Ожидался 1 заблокированный источник, получено %d", len(limited))
	}

	source, exists := limited[testSSRC]
	if !exists {
		t.Errorf("Источник %x должен быть в списке заблокированных", testSSRC)
	} else if !source.RateLimited {
		t.Error("Источник должен быть помечен как RateLimited")
	}

	t.Logf("✅ Rate limiting корректно работает: заблокировано %d вызовов", 
		atomic.LoadInt32(&rateLimitedCount))
}

// TestRateLimitingWindowReset тестирует сброс окна rate limiting
func TestRateLimitingWindowReset(t *testing.T) {
	config := SourceManagerConfig{
		MaxPacketsPerSecond: 3,                       // Максимум 3 пакета в секунду
		RateLimitWindow:     time.Millisecond * 100,  // Короткое окно для тестирования
	}

	sm := NewSourceManager(config)
	defer sm.Stop()

	testSSRC := uint32(0x87654321)

	// Отправляем пакеты до лимита
	for i := 0; i < 3; i++ {
		packet := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    0,
				SequenceNumber: uint16(i + 1),
				Timestamp:      uint32(i * 160),
				SSRC:           testSSRC,
			},
			Payload: make([]byte, 160),
		}

		source := sm.UpdateFromPacket(packet)
		if source == nil {
			t.Errorf("Пакет %d должен быть принят", i+1)
		}
	}

	// Отправляем пакет сверх лимита
	packet := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    0,
			SequenceNumber: 4,
			Timestamp:      480,
			SSRC:           testSSRC,
		},
		Payload: make([]byte, 160),
	}

	source := sm.UpdateFromPacket(packet)
	if source != nil {
		t.Error("Пакет должен быть заблокирован (превышение лимита)")
	}

	// Ждем сброса окна
	time.Sleep(time.Millisecond * 150)

	// Отправляем пакет после сброса окна
	packet = &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    0,
			SequenceNumber: 5,
			Timestamp:      640,
			SSRC:           testSSRC,
		},
		Payload: make([]byte, 160),
	}

	source = sm.UpdateFromPacket(packet)
	if source == nil {
		t.Error("Пакет должен быть принят после сброса окна")
	}

	// Проверяем что RateLimited флаг сброшен
	if source != nil && source.RateLimited {
		t.Error("RateLimited флаг должен быть сброшен после окна")
	}

	t.Logf("✅ Сброс окна rate limiting работает корректно")
}

// TestMultipleSourcesRateLimiting тестирует rate limiting для множественных источников
func TestMultipleSourcesRateLimiting(t *testing.T) {
	var rateLimitedCount int32

	config := SourceManagerConfig{
		MaxPacketsPerSecond: 2,                   // Очень низкий лимит для тестирования
		RateLimitWindow:     time.Second,
		OnRateLimited: func(ssrc uint32, source *RemoteSource) {
			atomic.AddInt32(&rateLimitedCount, 1)
		},
	}

	sm := NewSourceManager(config)
	defer sm.Stop()

	sources := []uint32{0x11111111, 0x22222222, 0x33333333}

	// Отправляем пакеты от каждого источника
	for _, ssrc := range sources {
		// Отправляем пакеты в пределах лимита
		for i := 0; i < 2; i++ {
			packet := &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					PayloadType:    0,
					SequenceNumber: uint16(i + 1),
					Timestamp:      uint32(i * 160),
					SSRC:           ssrc,
				},
				Payload: make([]byte, 160),
			}

			source := sm.UpdateFromPacket(packet)
			if source == nil {
				t.Errorf("Пакет от источника %x должен быть принят", ssrc)
			}
		}

		// Отправляем пакет сверх лимита
		packet := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    0,
				SequenceNumber: 3,
				Timestamp:      320,
				SSRC:           ssrc,
			},
			Payload: make([]byte, 160),
		}

		source := sm.UpdateFromPacket(packet)
		if source != nil {
			t.Errorf("Пакет от источника %x должен быть заблокирован", ssrc)
		}
	}

	// Ждем обработки async callbacks
	time.Sleep(time.Millisecond * 20)

	// Проверяем что все источники заблокированы (каждый источник вызовет callback один раз)
	expectedCount := int32(len(sources))
	if atomic.LoadInt32(&rateLimitedCount) != expectedCount {
		t.Errorf("Ожидалось %d rate limiting callbacks, получено %d", 
			expectedCount, atomic.LoadInt32(&rateLimitedCount))
	}

	// Проверяем что каждый источник заблокирован отдельно
	limited := sm.GetRateLimitedSources()
	if len(limited) != len(sources) {
		t.Errorf("GetRateLimitedSources вернул %d источников, ожидалось %d", 
			len(limited), len(sources))
	}

	for _, ssrc := range sources {
		if source, exists := limited[ssrc]; !exists {
			t.Errorf("Источник %x должен быть заблокирован", ssrc)
		} else if !source.RateLimited {
			t.Errorf("Источник %x должен иметь RateLimited=true", ssrc)
		}
	}

	t.Logf("✅ Rate limiting для множественных источников работает корректно")
}

// TestRateLimitingDisabled тестирует отключение rate limiting
func TestRateLimitingDisabled(t *testing.T) {
	config := SourceManagerConfig{
		MaxPacketsPerSecond: 0,  // Отключено
		RateLimitWindow:     time.Second,
	}

	sm := NewSourceManager(config)
	defer sm.Stop()

	testSSRC := uint32(0x99999999)

	// Отправляем пакеты быстро (больше обычного лимита)
	for i := 0; i < 100; i++ {
		packet := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    0,
				SequenceNumber: uint16(i + 1),
				Timestamp:      uint32(i * 160),
				SSRC:           testSSRC,
			},
			Payload: make([]byte, 160),
		}

		source := sm.UpdateFromPacket(packet)
		if source == nil {
			t.Errorf("Пакет %d должен быть принят (rate limiting отключен)", i+1)
		}
	}

	// Проверяем что нет заблокированных источников
	limited := sm.GetRateLimitedSources()
	if len(limited) != 0 {
		t.Errorf("При отключенном rate limiting не должно быть заблокированных источников, получено %d", 
			len(limited))
	}

	t.Logf("✅ Отключение rate limiting работает корректно")
}

// TestRateLimitingIntegration тестирует интеграцию rate limiting с RTP Session
func TestRateLimitingIntegration(t *testing.T) {
	transport := NewMockTransport()
	transport.SetActive(true)

	var rateLimitedCount int32

	config := SessionConfig{
		PayloadType: PayloadTypePCMU,
		MediaType:   MediaTypeAudio,
		ClockRate:   8000,
		Transport:   transport,
		OnPacketReceived: func(packet *rtp.Packet, addr net.Addr) {
			// Этот callback не должен вызываться для заблокированных пакетов
		},
	}

	session, err := NewSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer session.Stop()

	// Настраиваем rate limiting через SourceManager
	if session.sourceManager != nil {
		// Обновляем конфигурацию source manager для тестирования
		session.sourceManager.maxPacketsPerSecond = 3
		session.sourceManager.rateLimitWindow = time.Second
		session.sourceManager.onRateLimited = func(ssrc uint32, source *RemoteSource) {
			atomic.AddInt32(&rateLimitedCount, 1)
		}
	}

	_ = session.Start()

	testSSRC := uint32(0x44444444)

	// Симулируем получение пакетов через транспорт
	for i := 0; i < 10; i++ {
		packet := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    0,
				SequenceNumber: uint16(i + 1),
				Timestamp:      uint32(i * 160),
				SSRC:           testSSRC,
			},
			Payload: make([]byte, 160),
		}

		transport.SimulateReceive(packet)
	}

	// Ждем обработки пакетов
	time.Sleep(time.Millisecond * 50)

	// Проверяем что rate limiting сработал
	if atomic.LoadInt32(&rateLimitedCount) == 0 {
		t.Error("Rate limiting должен сработать в интеграционном тесте")
	} else {
		t.Logf("✅ Rate limiting работает в интеграции с RTP Session")
	}
}