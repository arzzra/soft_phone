package mockTransport

import (
	"fmt"
	"testing"
)

func BenchmarkPacketTransmission(b *testing.B) {
	registry := NewRegistry()
	conn1 := registry.CreateConnection("conn1")
	conn2 := registry.CreateConnection("conn2")
	defer conn1.Close()
	defer conn2.Close()

	data := []byte("benchmark test data")
	buf := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := conn1.WriteTo(data, conn2.LocalAddr())
		if err != nil {
			b.Fatal(err)
		}
		_, _, err = conn2.ReadFrom(buf)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkConcurrentTransmission(b *testing.B) {
	registry := NewRegistry()
	registry.SetBufferSize(1000) // Большой буфер для бенчмарка

	numConns := 10
	conns := make([]*MockPacketConn, numConns)
	for i := 0; i < numConns; i++ {
		conns[i] = registry.CreateConnection(string(rune('A' + i)))
		defer conns[i].Close()
	}

	data := []byte("concurrent benchmark data")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		buf := make([]byte, 1024)
		idx := 0
		for pb.Next() {
			from := conns[idx%numConns]
			to := conns[(idx+1)%numConns]

			_, err := from.WriteTo(data, to.LocalAddr())
			if err != nil {
				b.Fatal(err)
			}
			_, _, err = to.ReadFrom(buf)
			if err != nil {
				b.Fatal(err)
			}

			idx++
		}
	})
}

func BenchmarkRegistryOperations(b *testing.B) {
	registry := NewRegistry()

	b.Run("CreateConnection", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			conn := registry.CreateConnection(string(rune(i%26 + 'A')))
			conn.Close()
		}
	})

	b.Run("GetConnection", func(b *testing.B) {
		// Создаем соединения
		for i := 0; i < 26; i++ {
			registry.CreateConnection(string(rune(i + 'A')))
		}
		defer registry.CloseAll()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = registry.GetConnection(string(rune(i%26 + 'A')))
		}
	})

	b.Run("ListConnections", func(b *testing.B) {
		// Создаем соединения
		for i := 0; i < 100; i++ {
			registry.CreateConnection(fmt.Sprintf("conn%d", i))
		}
		defer registry.CloseAll()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = registry.ListConnections()
		}
	})
}
