package rtp

import (
	"runtime"
	"testing"
	
	"github.com/pion/rtp"
)

// Simple RTP Transport Tests

func TestSimpleRTPTransportCreation(t *testing.T) {
	// Проверяем, что можем создать RTP транспорт
	config := TransportConfig{
		LocalAddr:  "127.0.0.1:5004",
		RemoteAddr: "127.0.0.1:5006",
		BufferSize: 1500,
	}
	
	transport, err := NewUDPTransport(config)
	if err != nil {
		t.Fatalf("Should create RTP transport: %v", err)
	}
	defer transport.Close()
	
	// Проверяем базовые свойства
	if transport.LocalAddr() == nil {
		t.Error("Transport should have local address")
	}
	
	if transport.RemoteAddr() == nil {
		t.Error("Transport should have remote address")
	}
}

func TestSimpleRTPPacketSend(t *testing.T) {
	// Тестируем отправку RTP пакета
	config := TransportConfig{
		LocalAddr:  "127.0.0.1:5004",
		RemoteAddr: "127.0.0.1:5006",
		BufferSize: 1500,
	}
	
	transport, err := NewUDPTransport(config)
	if err != nil {
		t.Fatalf("Should create RTP transport: %v", err)
	}
	defer transport.Close()
	
	// Создаем простой RTP пакет
	packet := createSimpleRTPPacket()
	
	// Пытаемся отправить пакет
	err = transport.Send(packet)
	if err != nil {
		// Ожидаем сетевую ошибку, но не ошибку формата
		if !isRTPNetworkError(err) {
			t.Errorf("Should handle RTP packet, got non-network error: %v", err)
		}
	}
}

func TestRTPTransportResourceCleanup(t *testing.T) {
	// Проверяем очистку ресурсов
	const numTransports = 5
	
	for i := 0; i < numTransports; i++ {
		config := TransportConfig{
			LocalAddr:  "127.0.0.1:0", // Автоматический порт
			RemoteAddr: "127.0.0.1:5006",
			BufferSize: 1500,
		}
		
		transport, err := NewUDPTransport(config)
		if err != nil {
			t.Fatalf("Should create RTP transport %d: %v", i, err)
		}
		
		// Проверяем, что транспорт работает
		if transport.LocalAddr() == nil {
			t.Errorf("Transport %d should have valid local address", i)
		}
		
		err = transport.Close()
		if err != nil {
			t.Errorf("Should close RTP transport %d: %v", i, err)
		}
	}
}

func TestRTPPlatformSupport(t *testing.T) {
	// Простая проверка платформенной поддержки
	config := TransportConfig{
		LocalAddr:  "127.0.0.1:0",
		RemoteAddr: "127.0.0.1:5006",
		BufferSize: 1500,
	}
	
	transport, err := NewUDPTransport(config)
	if err != nil {
		t.Fatalf("Should create RTP transport on %s: %v", runtime.GOOS, err)
	}
	defer transport.Close()
	
	// Проверяем, что адреса валидны
	localAddr := transport.LocalAddr()
	remoteAddr := transport.RemoteAddr()
	
	if localAddr == nil {
		t.Error("Local address should not be nil")
	}
	
	if remoteAddr == nil {
		t.Error("Remote address should not be nil")
	}
	
	t.Logf("Platform: %s, Local: %v, Remote: %v", runtime.GOOS, localAddr, remoteAddr)
}

// Benchmarks

func BenchmarkSimpleRTPTransportCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		config := TransportConfig{
			LocalAddr:  "127.0.0.1:0",
			RemoteAddr: "127.0.0.1:5006",
			BufferSize: 1500,
		}
		
		transport, err := NewUDPTransport(config)
		if err != nil {
			b.Fatalf("Failed to create RTP transport: %v", err)
		}
		transport.Close()
	}
}

func BenchmarkSimpleRTPPacketWrite(b *testing.B) {
	config := TransportConfig{
		LocalAddr:  "127.0.0.1:0",
		RemoteAddr: "127.0.0.1:5006",
		BufferSize: 1500,
	}
	
	transport, err := NewUDPTransport(config)
	if err != nil {
		b.Fatalf("Failed to create RTP transport: %v", err)
	}
	defer transport.Close()
	
	packet := createSimpleRTPPacket()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		transport.Send(packet)
	}
}

// Вспомогательные функции

func createSimpleRTPPacket() *rtp.Packet {
	// Создаем минимальный валидный RTP пакет
	return &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    0, // G.711 μ-law
			SequenceNumber: 1,
			Timestamp:      160,
			SSRC:           0x12345678,
		},
		Payload: []byte("test payload"),
	}
}

func isRTPNetworkError(err error) bool {
	if err == nil {
		return false
	}
	
	errStr := err.Error()
	networkErrors := []string{
		"connection refused",
		"network is unreachable",
		"no route to host",
		"timeout",
		"permission denied",
		"address already in use",
	}
	
	for _, netErr := range networkErrors {
		if contains(errStr, netErr) {
			return true
		}
	}
	
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || 
		(len(substr) <= len(s) && s[len(s)-len(substr):] == substr) ||
		(len(substr) <= len(s) && s[:len(substr)] == substr) ||
		(len(substr) < len(s) && indexOf(s, substr) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}