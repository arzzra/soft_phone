// Package mockTransport предоставляет in-memory реализацию net.PacketConn для тестирования.
//
// Этот пакет реализует полнофункциональный транспорт, работающий через память,
// что позволяет тестировать сетевой код без использования реальных сетевых соединений.
//
// Основные возможности:
//   - Полная совместимость с интерфейсом net.PacketConn
//   - Потокобезопасная реализация
//   - Поддержка deadlines и timeouts
//   - Централизованный Registry для управления соединениями
//   - Эмуляция сетевых ошибок для тестирования
//
// Пример использования:
//
//	registry := mockTransport.NewRegistry()
//
//	// Создание двух соединений
//	conn1 := registry.CreateConnection("addr1")
//	conn2 := registry.CreateConnection("addr2")
//
//	// Отправка данных от conn1 к conn2
//	_, err := conn1.WriteTo([]byte("Hello"), conn2.LocalAddr())
//
//	// Чтение данных на conn2
//	buf := make([]byte, 1024)
//	n, addr, err := conn2.ReadFrom(buf)
package mockTransport
