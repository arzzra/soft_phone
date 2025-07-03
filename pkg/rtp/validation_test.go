// validation_test.go - тесты для проверки input validation и DoS protection
package rtp

import (
	"testing"

	"github.com/pion/rtp"
)

// TestValidatePacketSize тестирует валидацию размера пакетов
func TestValidatePacketSize(t *testing.T) {
	tests := []struct {
		name        string
		size        int
		shouldError bool
		description string
	}{
		{
			name:        "Минимальный валидный размер",
			size:        MinRTPPacketSize,
			shouldError: false,
			description: "Пакет минимального размера должен быть принят",
		},
		{
			name:        "Максимальный валидный размер",
			size:        MaxRTPPacketSize,
			shouldError: false,
			description: "Пакет максимального размера должен быть принят",
		},
		{
			name:        "Пакет слишком мал",
			size:        MinRTPPacketSize - 1,
			shouldError: true,
			description: "Пакет меньше минимального размера должен быть отклонен",
		},
		{
			name:        "Пакет слишком велик",
			size:        MaxRTPPacketSize + 1,
			shouldError: true,
			description: "Пакет больше максимального размера должен быть отклонен (DoS protection)",
		},
		{
			name:        "Нулевой размер",
			size:        0,
			shouldError: true,
			description: "Пакет нулевого размера должен быть отклонен",
		},
		{
			name:        "Стандартный аудио пакет",
			size:        172, // 12 (header) + 160 (G.711 20ms)
			shouldError: false,
			description: "Стандартный G.711 пакет должен быть принят",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Тест валидации размера: %s", tt.description)

			err := validatePacketSize(tt.size)

			if tt.shouldError {
				if err == nil {
					t.Errorf("Ожидалась ошибка для размера %d, но валидация прошла", tt.size)
				} else {
					t.Logf("✅ Корректно отклонен пакет размера %d: %s", tt.size, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Неожиданная ошибка для валидного размера %d: %v", tt.size, err)
				} else {
					t.Logf("✅ Корректно принят пакет размера %d", tt.size)
				}
			}
		})
	}
}

// TestValidateRTPHeader тестирует валидацию RTP заголовков
func TestValidateRTPHeader(t *testing.T) {
	tests := []struct {
		name        string
		header      rtp.Header
		shouldError bool
		description string
	}{
		{
			name: "Валидный RTP заголовок",
			header: rtp.Header{
				Version:     2,
				PayloadType: 0, // PCMU
				SSRC:        0x12345678,
			},
			shouldError: false,
			description: "Стандартный RTP заголовок должен быть принят",
		},
		{
			name: "Неверная версия RTP",
			header: rtp.Header{
				Version:     1, // Неверная версия
				PayloadType: 0,
				SSRC:        0x12345678,
			},
			shouldError: true,
			description: "RTP версии 1 должна быть отклонена",
		},
		{
			name: "Невалидный payload type",
			header: rtp.Header{
				Version:     2,
				PayloadType: 128, // Больше максимального
				SSRC:        0x12345678,
			},
			shouldError: true,
			description: "Payload type > 127 должен быть отклонен",
		},
		{
			name: "Максимальный валидный payload type",
			header: rtp.Header{
				Version:     2,
				PayloadType: 127, // Максимальный валидный
				SSRC:        0x12345678,
			},
			shouldError: false,
			description: "Payload type 127 должен быть принят",
		},
		{
			name: "G.722 payload type",
			header: rtp.Header{
				Version:     2,
				PayloadType: 9, // G.722
				SSRC:        0x87654321,
			},
			shouldError: false,
			description: "G.722 payload type должен быть принят",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Тест валидации заголовка: %s", tt.description)

			err := validateRTPHeader(&tt.header)

			if tt.shouldError {
				if err == nil {
					t.Errorf("Ожидалась ошибка для заголовка %+v, но валидация прошла", tt.header)
				} else {
					t.Logf("✅ Корректно отклонен заголовок: %s", err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Неожиданная ошибка для валидного заголовка %+v: %v", tt.header, err)
				} else {
					t.Logf("✅ Корректно принят заголовок версии %d, PT %d", 
						tt.header.Version, tt.header.PayloadType)
				}
			}
		})
	}
}

// TestUDPTransportValidation тестирует валидацию на уровне UDP транспорта
func TestUDPTransportValidation(t *testing.T) {
	// Создаем реальный UDP транспорт для тестирования валидации
	config := TransportConfig{
		LocalAddr:  ":0", // Автоматический выбор порта
		BufferSize: 1500,
	}
	
	transport, err := NewUDPTransport(config)
	if err != nil {
		t.Skipf("Не удалось создать UDP транспорт для тестирования: %v", err)
		return
	}
	defer transport.Close()
	
	// Устанавливаем remote addr для возможности отправки
	err = transport.SetRemoteAddr("127.0.0.1:12345")
	if err != nil {
		t.Skipf("Не удалось установить remote addr: %v", err)
		return
	}

	tests := []struct {
		name        string
		packet      *rtp.Packet
		shouldError bool
		description string
	}{
		{
			name: "Валидный пакет",
			packet: &rtp.Packet{
				Header: rtp.Header{
					Version:     2,
					PayloadType: 0,
					SSRC:        0x12345678,
				},
				Payload: make([]byte, 160), // G.711 20ms
			},
			shouldError: false,
			description: "Стандартный валидный пакет должен быть отправлен",
		},
		{
			name: "Пакет с неверной версией",
			packet: &rtp.Packet{
				Header: rtp.Header{
					Version:     1, // Неверная версия
					PayloadType: 0,
					SSRC:        0x12345678,
				},
				Payload: make([]byte, 160),
			},
			shouldError: true,
			description: "Пакет с неверной версией должен быть отклонен",
		},
		{
			name: "Пакет с невалидным payload type",
			packet: &rtp.Packet{
				Header: rtp.Header{
					Version:     2,
					PayloadType: 200, // Невалидный
					SSRC:        0x12345678,
				},
				Payload: make([]byte, 160),
			},
			shouldError: true,
			description: "Пакет с невалидным payload type должен быть отклонен",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Тест валидации транспорта: %s", tt.description)

			err := transport.Send(tt.packet)

			if tt.shouldError {
				if err == nil {
					t.Errorf("Ожидалась ошибка для пакета %+v, но отправка прошла", tt.packet.Header)
				} else {
					t.Logf("✅ Корректно отклонен пакет: %s", err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Неожиданная ошибка для валидного пакета %+v: %v", tt.packet.Header, err)
				} else {
					t.Logf("✅ Корректно отправлен пакет версии %d, PT %d", 
						tt.packet.Header.Version, tt.packet.Header.PayloadType)
				}
			}
		})
	}
}

// TestMalformedPacketsHandling тестирует обработку поврежденных пакетов
func TestMalformedPacketsHandling(t *testing.T) {
	// Тест будет расширен при добавлении реального UDP транспорта
	// В текущем виде MockTransport не тестирует демаршалинг raw bytes
	
	t.Log("Тест обработки поврежденных пакетов - базовая проверка")
	
	// Проверяем что очень маленькие и очень большие размеры отклоняются
	if err := validatePacketSize(1); err == nil {
		t.Error("Размер 1 байт должен быть отклонен")
	}
	
	if err := validatePacketSize(10000); err == nil {
		t.Error("Размер 10000 байт должен быть отклонен")
	}
	
	t.Log("✅ Базовая защита от поврежденных пакетов работает")
}