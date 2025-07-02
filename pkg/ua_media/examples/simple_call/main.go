package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/arzzra/soft_phone/pkg/dialog"
	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/rtp"
	"github.com/arzzra/soft_phone/pkg/ua_media"
	"github.com/emiago/sipgo/sip"
)

func main() {
	fmt.Println("🎯 UA Media Package - Simple Call Example")
	fmt.Println("========================================")

	// Создаем контекст с отменой
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Обработка сигналов для graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Создаем SIP стек
	stackConfig := &dialog.StackConfig{
		Transport: &dialog.TransportConfig{
			Protocol: "udp",
			Address:  "0.0.0.0",
			Port:     5060,
		},
		UserAgent:  "UA-Media-Example/1.0",
		MaxDialogs: 100,
	}

	stack, err := dialog.NewStack(stackConfig)
	if err != nil {
		log.Fatalf("Ошибка создания SIP стека: %v", err)
	}

	// Запускаем стек
	go func() {
		if err := stack.Start(ctx); err != nil {
			log.Printf("Ошибка запуска стека: %v", err)
		}
	}()

	// Даем время на запуск
	time.Sleep(100 * time.Millisecond)

	// Создаем конфигурацию для UA Media
	uaConfig := ua_media.DefaultConfig()
	uaConfig.Stack = stack
	uaConfig.SessionName = "Simple Call Example"
	uaConfig.UserAgent = "UA-Media-Example/1.0"

	// Настраиваем медиа
	uaConfig.MediaConfig.PayloadType = media.PayloadTypePCMU
	uaConfig.MediaConfig.Direction = media.DirectionSendRecv
	uaConfig.MediaConfig.DTMFEnabled = true

	// Настраиваем колбэки
	uaConfig.Callbacks = ua_media.SessionCallbacks{
		OnStateChanged: func(oldState, newState dialog.DialogState) {
			fmt.Printf("📞 Состояние изменилось: %s → %s\n", oldState, newState)
		},

		OnMediaStarted: func() {
			fmt.Println("🎵 Медиа сессия запущена")
		},

		OnMediaStopped: func() {
			fmt.Println("🛑 Медиа сессия остановлена")
		},

		OnAudioReceived: func(data []byte, pt media.PayloadType, ptime time.Duration) {
			fmt.Printf("🔊 Получено аудио: %d байт, codec %d, ptime %v\n", len(data), pt, ptime)
		},

		OnDTMFReceived: func(event media.DTMFEvent) {
			fmt.Printf("☎️  DTMF: %s (длительность: %v)\n", event.Digit, event.Duration)
		},

		OnError: func(err error) {
			fmt.Printf("❌ Ошибка: %v\n", err)
		},
	}

	// Обработчик входящих вызовов
	stack.OnIncomingDialog(func(incomingDialog dialog.IDialog) {
		fmt.Println("\n📞 Входящий вызов!")

		// Создаем UA Media сессию для входящего вызова
		session, err := ua_media.NewIncomingCall(ctx, incomingDialog, uaConfig)
		if err != nil {
			log.Printf("Ошибка создания входящей сессии: %v", err)
			return
		}

		// Автоматически принимаем вызов через 2 секунды
		go func() {
			time.Sleep(2 * time.Second)
			fmt.Println("✅ Принимаем вызов...")

			if err := session.Accept(ctx); err != nil {
				log.Printf("Ошибка принятия вызова: %v", err)
				return
			}

			// Демонстрация отправки DTMF
			go func() {
				time.Sleep(5 * time.Second)
				fmt.Println("📱 Отправляем DTMF последовательность: 1234")

				digits := []rtp.DTMFDigit{
					rtp.DTMFDigit1, rtp.DTMFDigit2,
					rtp.DTMFDigit3, rtp.DTMFDigit4,
				}

				for _, digit := range digits {
					if err := session.SendDTMF(digit, 160*time.Millisecond); err != nil {
						log.Printf("Ошибка отправки DTMF: %v", err)
					}
					time.Sleep(500 * time.Millisecond)
				}
			}()
		}()
	})

	// Пример исходящего вызова
	if len(os.Args) > 1 {
		targetURI := os.Args[1]
		fmt.Printf("\n📞 Исходящий вызов на: %s\n", targetURI)

		// Парсим SIP URI
		sipURI, err := ua_media.ParseSIPURI(targetURI)
		if err != nil {
			log.Fatalf("Ошибка парсинга URI: %v", err)
		}

		// Создаем исходящий вызов
		session, err := ua_media.NewOutgoingCall(ctx, sipURI, uaConfig)
		if err != nil {
			log.Fatalf("Ошибка создания исходящего вызова: %v", err)
		}

		// Ожидаем ответ
		fmt.Println("⏳ Ожидаем ответ...")
		if err := session.WaitAnswer(ctx); err != nil {
			log.Printf("Вызов отклонен: %v", err)
			session.Close()
			return
		}

		fmt.Println("✅ Вызов установлен!")

		// Демонстрация отправки аудио
		go func() {
			ticker := time.NewTicker(20 * time.Millisecond)
			defer ticker.Stop()

			// Генерируем тишину (160 байт для 20мс PCMU)
			silenceData := make([]byte, 160)

			for {
				select {
				case <-ticker.C:
					if session.State() == dialog.DialogStateEstablished {
						if err := session.SendAudio(silenceData); err != nil {
							log.Printf("Ошибка отправки аудио: %v", err)
							return
						}
					}
				case <-ctx.Done():
					return
				}
			}
		}()

		// Завершаем вызов через 30 секунд
		go func() {
			time.Sleep(30 * time.Second)
			if session.State() == dialog.DialogStateEstablished {
				fmt.Println("⏰ Завершаем вызов по таймеру...")
				if err := session.Bye(ctx); err != nil {
					log.Printf("Ошибка завершения вызова: %v", err)
				}
			}
		}()
	} else {
		fmt.Println("\n💡 Использование:")
		fmt.Println("   - Для исходящего вызова: go run main.go sip:user@host:port")
		fmt.Println("   - Для приема входящих: go run main.go")
		fmt.Println("\n⏳ Ожидаем входящие вызовы на порту 5060...")
	}

	// Ожидаем сигнал завершения
	<-sigChan
	fmt.Println("\n🛑 Завершение работы...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := stack.Shutdown(shutdownCtx); err != nil {
		log.Printf("Ошибка завершения стека: %v", err)
	}

	fmt.Println("👋 Пример завершен")
}

// Дополнительные примеры использования

// exampleWithCallbacks демонстрирует расширенное использование колбэков
func exampleWithCallbacks() {
	config := ua_media.DefaultConfig()

	config.Callbacks = ua_media.SessionCallbacks{
		OnEvent: func(event ua_media.SessionEvent) {
			// Обработка всех событий в одном месте
			switch event.Type {
			case ua_media.EventStateChanged:
				state := event.Data.(dialog.DialogState)
				fmt.Printf("Событие: смена состояния на %s\n", state)

			case ua_media.EventSDPReceived:
				sdp := event.Data.(*sdp.SessionDescription)
				fmt.Printf("Событие: получен SDP с %d медиа\n", len(sdp.MediaDescriptions))

			case ua_media.EventError:
				fmt.Printf("Событие: ошибка %v\n", event.Error)
			}
		},

		OnRawPacketReceived: func(packet *rtp.Packet) {
			// Обработка сырых RTP пакетов для записи или анализа
			fmt.Printf("RTP пакет: seq=%d, timestamp=%d, payload=%d байт\n",
				packet.SequenceNumber, packet.Timestamp, len(packet.Payload))
		},
	}
}

// exampleWithExtendedConfig демонстрирует использование расширенной конфигурации
func exampleWithExtendedConfig() {
	config := ua_media.DefaultExtendedConfig()

	// Настройка качества обслуживания
	config.QoS.DSCP = 46 // EF для VoIP
	config.QoS.JitterBufferSize = 100 * time.Millisecond
	config.QoS.PacketLossConcealment = true

	// Настройка безопасности
	config.Security.SRTP = true
	config.Security.SRTPProfile = "AES_CM_128_HMAC_SHA1_80"

	// Настройка записи
	config.RecordingEnabled = true
	config.RecordingPath = "./recordings"

	// Настройка медиа предпочтений
	config.MediaPreferences.PreferredCodec = rtp.PayloadTypeG722
	config.MediaPreferences.EchoCancellation = true
	config.MediaPreferences.NoiseSuppression = true
}
