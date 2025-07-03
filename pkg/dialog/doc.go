// Package dialog - Расширенные примеры использования
//
// Этот файл содержит детальные примеры advanced use cases пакета dialog.

package dialog

// Пример 1: Полный жизненный цикл SIP вызова с обработкой ошибок
//
//	func handleOutgoingCall(ctx context.Context, stack *Stack, target string) error {
//		// Парсинг URI
//		targetURI, err := sip.ParseUri(target)
//		if err != nil {
//			return fmt.Errorf("invalid target URI: %w", err)
//		}
//
//		// Создание SDP предложения
//		sdpOffer := `v=0
//	o=- 1234567890 1234567890 IN IP4 192.168.1.100
//	s=SIP Call
//	c=IN IP4 192.168.1.100
//	t=0 0
//	m=audio 5004 RTP/AVP 0 8
//	a=rtpmap:0 PCMU/8000
//	a=rtpmap:8 PCMA/8000`
//		
//		body := NewBody("application/sdp", []byte(sdpOffer))
//		opts := InviteOpts{Body: body}
//
//		// Создание исходящего диалога
//		dialog, err := stack.NewInvite(ctx, targetURI, opts)
//		if err != nil {
//			return fmt.Errorf("failed to create dialog: %w", err)
//		}
//
//		// Настройка колбэков состояния
//		dialog.OnStateChange(func(state DialogState) {
//			log.Printf("Dialog state changed to: %s", state)
//			switch state {
//			case DialogStateRinging:
//				log.Println("Call is ringing...")
//			case DialogStateEstablished:
//				log.Println("Call established successfully")
//			case DialogStateTerminated:
//				log.Println("Call terminated")
//			}
//		})
//
//		// Ожидание ответа с таймаутом
//		answerCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
//		defer cancel()
//
//		if err := dialog.WaitAnswer(answerCtx); err != nil {
//			if errors.Is(err, context.DeadlineExceeded) {
//				log.Println("Call timeout - no answer")
//				dialog.Cancel(ctx, "Request Timeout")
//			}
//			return fmt.Errorf("call failed: %w", err)
//		}
//
//		// Вызов установлен - держим активным 30 секунд
//		log.Println("Call is active...")
//		time.Sleep(30 * time.Second)
//
//		// Завершение вызова
//		if err := dialog.Bye(ctx, "Normal call termination"); err != nil {
//			log.Printf("Error sending BYE: %v", err)
//		}
//
//		return nil
//	}

// Пример 2: Обработка входящих вызовов с автоматическим принятием
//
//	func setupIncomingCallHandler(stack *Stack) {
//		stack.OnIncomingDialog(func(dialog IDialog) {
//			log.Printf("Incoming call from: %s", dialog.RemoteURI())
//			
//			// Асинхронная обработка входящего вызова
//			go handleIncomingCall(dialog)
//		})
//	}
//
//	func handleIncomingCall(dialog IDialog) {
//		ctx := context.Background()
//		
//		// Отправляем 180 Ringing (автоматически)
//		log.Println("Sending 180 Ringing...")
//		
//		// Имитация времени на принятие решения
//		time.Sleep(2 * time.Second)
//		
//		// Создание SDP ответа
//		sdpAnswer := `v=0
//	o=- 9876543210 9876543210 IN IP4 192.168.1.200  
//	s=SIP Call
//	c=IN IP4 192.168.1.200
//	t=0 0
//	m=audio 5006 RTP/AVP 0 8
//	a=rtpmap:0 PCMU/8000
//	a=rtpmap:8 PCMA/8000`
//
//		// Принятие вызова с SDP
//		err := dialog.Accept(ctx, func(resp *sip.Response) {
//			resp.SetBody([]byte(sdpAnswer))
//			resp.AppendHeader(sip.NewHeader("Content-Type", "application/sdp"))
//		})
//		
//		if err != nil {
//			log.Printf("Failed to accept call: %v", err)
//			dialog.Reject(ctx, 500, "Internal Server Error")
//			return
//		}
//		
//		log.Println("Call accepted and established")
//		
//		// Обработка активного вызова
//		// В реальном приложении здесь был бы RTP/media processing
//		time.Sleep(60 * time.Second)
//		
//		log.Println("Auto-terminating call")
//		dialog.Bye(ctx, "Session timeout")
//	}

// Пример 3: Перевод вызова (Call Transfer) с REFER
//
//	func transferCall(ctx context.Context, dialog *Dialog, transferTarget string) error {
//		// Парсинг URI цели перевода
//		targetURI, err := sip.ParseUri(transferTarget)
//		if err != nil {
//			return fmt.Errorf("invalid transfer target: %w", err)
//		}
//
//		log.Printf("Transferring call to: %s", transferTarget)
//
//		// Отправка REFER запроса
//		opts := ReferOpts{
//			ReferTo: targetURI,
//			// Можно добавить Replaces для attended transfer
//		}
//		
//		err = dialog.SendRefer(ctx, targetURI, opts)
//		if err != nil {
//			return fmt.Errorf("failed to send REFER: %w", err)
//		}
//
//		// Ожидание принятия REFER
//		subscription, err := dialog.WaitRefer(ctx)
//		if err != nil {
//			return fmt.Errorf("REFER was rejected: %w", err)
//		}
//
//		log.Printf("REFER accepted, subscription ID: %s", subscription.ID)
//
//		// Мониторинг статуса перевода через NOTIFY
//		subscription.OnNotify(func(status string) {
//			log.Printf("Transfer status: %s", status)
//			if status == "200 OK" {
//				log.Println("Transfer completed successfully")
//				// Можно завершить исходный диалог
//				dialog.Bye(ctx, "Call transferred")
//			}
//		})
//
//		return nil
//	}

// Пример 4: Высоконагруженная обработка множественных диалогов
//
//	func handleHighLoad(ctx context.Context) error {
//		// Конфигурация для высоких нагрузок
//		config := &StackConfig{
//			Transport: &TransportConfig{
//				Protocol: "udp",
//				Address:  "0.0.0.0",
//				Port:     5060,
//			},
//			UserAgent:  "HighLoadServer/1.0",
//			MaxDialogs: 10000, // Поддержка до 10k одновременных диалогов
//			MetricsConfig: &MetricsConfig{
//				Enabled:  true,
//				Interval: 30 * time.Second,
//			},
//		}
//
//		stack, err := NewStack(config)
//		if err != nil {
//			return fmt.Errorf("failed to create stack: %w", err)
//		}
//
//		// Graceful shutdown
//		defer func() {
//			log.Println("Shutting down stack...")
//			shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//			defer cancel()
//			stack.Shutdown(shutdownCtx)
//		}()
//
//		// Обработчик входящих вызовов с ограничением rate
//		limiter := rate.NewLimiter(rate.Limit(100), 1000) // 100 RPS, burst 1000
//		
//		stack.OnIncomingDialog(func(dialog IDialog) {
//			if !limiter.Allow() {
//				log.Println("Rate limit exceeded, rejecting call")
//				dialog.Reject(ctx, 503, "Service Unavailable")
//				return
//			}
//			
//			// Обработка в отдельной горутине
//			go processCallAsync(ctx, dialog)
//		})
//
//		// Мониторинг метрик
//		go func() {
//			ticker := time.NewTicker(30 * time.Second)
//			defer ticker.Stop()
//			
//			for {
//				select {
//				case <-ticker.C:
//					metrics := stack.GetMetrics()
//					log.Printf("Active dialogs: %d, Success rate: %.2f%%, Memory: %s",
//						metrics.ActiveDialogs, metrics.SuccessRate, metrics.MemoryUsage)
//				case <-ctx.Done():
//					return
//				}
//			}
//		}()
//
//		// Запуск стека
//		return stack.Start(ctx)
//	}
//
//	func processCallAsync(ctx context.Context, dialog IDialog) {
//		defer func() {
//			if r := recover(); r != nil {
//				log.Printf("Panic in call processing: %v", r)
//				dialog.Reject(ctx, 500, "Internal Server Error")
//			}
//		}()
//
//		// Быстрое принятие вызова
//		err := dialog.Accept(ctx)
//		if err != nil {
//			log.Printf("Failed to accept call: %v", err)
//			return
//		}
//
//		// Короткий активный период для высокого throughput
//		time.Sleep(5 * time.Second)
//		
//		dialog.Bye(ctx, "Session complete")
//	}

// Пример 5: Robust error handling и retry логика
//
//	func robustCallAttempt(ctx context.Context, stack *Stack, target string, maxRetries int) error {
//		var lastErr error
//		
//		for attempt := 0; attempt < maxRetries; attempt++ {
//			if attempt > 0 {
//				backoff := time.Duration(attempt*attempt) * time.Second
//				log.Printf("Retry attempt %d after %v", attempt+1, backoff)
//				time.Sleep(backoff)
//			}
//
//			err := attemptCall(ctx, stack, target)
//			if err == nil {
//				return nil // Успех
//			}
//
//			lastErr = err
//			
//			// Анализ типа ошибки для принятия решения о retry
//			if dialogErr, ok := err.(*DialogError); ok {
//				switch dialogErr.Category {
//				case ErrorCategoryTransport:
//					log.Printf("Transport error (will retry): %v", err)
//					continue
//				case ErrorCategoryProtocol:
//					if dialogErr.Code == "408" || dialogErr.Code == "503" {
//						log.Printf("Temporary error (will retry): %v", err)
//						continue
//					}
//					log.Printf("Permanent protocol error (no retry): %v", err)
//					return err
//				case ErrorCategorySystem:
//					log.Printf("System error (no retry): %v", err)
//					return err
//				}
//			}
//			
//			// Неизвестная ошибка - retry
//			log.Printf("Unknown error (will retry): %v", err)
//		}
//		
//		return fmt.Errorf("call failed after %d attempts, last error: %w", maxRetries, lastErr)
//	}
//
//	func attemptCall(ctx context.Context, stack *Stack, target string) error {
//		targetURI, err := sip.ParseUri(target)
//		if err != nil {
//			return fmt.Errorf("invalid target: %w", err)
//		}
//
//		dialog, err := stack.NewInvite(ctx, targetURI, InviteOpts{})
//		if err != nil {
//			return err
//		}
//
//		// Короткий таймаут для быстрого retry
//		callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
//		defer cancel()
//
//		err = dialog.WaitAnswer(callCtx)
//		if err != nil {
//			dialog.Cancel(ctx, "Timeout")
//			return err
//		}
//
//		// Быстрое завершение для тестирования
//		time.Sleep(time.Second)
//		return dialog.Bye(ctx, "Test complete")
//	}