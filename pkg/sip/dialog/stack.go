package dialog

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/core/builder"
	"github.com/arzzra/soft_phone/pkg/sip/core/types"
	"github.com/arzzra/soft_phone/pkg/sip/transaction"
	"github.com/arzzra/soft_phone/pkg/sip/transport"
)

// Stack реализация SIP стека для управления диалогами
type Stack struct {
	// Транспорт и транзакции
	transportManager transport.TransportManager
	txManager        transaction.TransactionManager

	// Диалоги
	dialogManager *DialogManager

	// Обработчики
	incomingDialogHandler func(IDialog)
	requestHandlers       map[string]RequestHandler
	handlersMutex         sync.RWMutex

	// Состояние
	running bool
	runMutex sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	// Настройки
	localAddress string
	localPort    int
}

// NewStack создает новый SIP стек
func NewStack(transportManager transport.TransportManager, localAddress string, localPort int) *Stack {
	return &Stack{
		transportManager: transportManager,
		dialogManager:    NewDialogManager(),
		requestHandlers:  make(map[string]RequestHandler),
		localAddress:     localAddress,
		localPort:        localPort,
	}
}

// Start запускает listener'ы и обработку сообщений
func (s *Stack) Start(ctx context.Context) error {
	s.runMutex.Lock()
	if s.running {
		s.runMutex.Unlock()
		return fmt.Errorf("stack already running")
	}
	s.running = true
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.runMutex.Unlock()

	// Создаем менеджер транзакций
	s.txManager = transaction.NewManager(s.transportManager)

	// Устанавливаем обработчики для транзакционного слоя
	s.txManager.OnRequest(s.handleIncomingRequest)
	s.txManager.OnResponse(s.handleIncomingResponse)

	// Запускаем обработку в отдельной горутине
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		<-s.ctx.Done()
		// Контекст отменен, завершаем работу
	}()

	return nil
}

// Shutdown останавливает стек и завершает все диалоги
func (s *Stack) Shutdown(ctx context.Context) error {
	s.runMutex.Lock()
	if !s.running {
		s.runMutex.Unlock()
		return fmt.Errorf("stack not running")
	}
	s.running = false
	s.runMutex.Unlock()

	// Завершаем все активные диалоги
	dialogs := s.dialogManager.GetAll()

	// Отправляем BYE для всех установленных диалогов
	for _, d := range dialogs {
		if d.State() == DialogStateEstablished {
			if err := d.Bye(ctx, "Stack shutdown"); err != nil {
				// Логируем ошибку, но продолжаем
				fmt.Printf("Failed to send BYE for dialog %s: %v\n", d.Key(), err)
			}
		}
		// Закрываем диалог
		d.Close()
	}

	// Останавливаем менеджер транзакций
	if s.txManager != nil {
		if err := s.txManager.Close(); err != nil {
			return fmt.Errorf("failed to close transaction manager: %w", err)
		}
	}

	// Отменяем контекст
	if s.cancel != nil {
		s.cancel()
	}

	// Ждем завершения всех горутин
	s.wg.Wait()

	// Очищаем диалоги
	s.dialogManager.Clear()

	return nil
}

// NewInvite создает исходящий INVITE и новый диалог
func (s *Stack) NewInvite(ctx context.Context, target URI, opts InviteOpts) (IDialog, error) {
	s.runMutex.RLock()
	if !s.running {
		s.runMutex.RUnlock()
		return nil, fmt.Errorf("stack not running")
	}
	s.runMutex.RUnlock()

	// Генерируем Call-ID и From tag
	callID := GenerateCallID()
	fromTag := GenerateLocalTag()

	// Создаем INVITE запрос
	fromURI := types.NewSipURI("", s.localAddress)
	fromURI.SetPort(s.localPort)
	
	// Создаем адреса From и To
	fromAddr := types.NewAddress("", fromURI)
	fromAddr.SetParameter("tag", fromTag)
	toAddr := types.NewAddress("", target)
	
	// Используем helper функцию CreateRequest из builder
	reqBuilder := builder.CreateRequest(types.MethodINVITE, fromAddr, toAddr, callID, 1)
	
	// Добавляем Contact
	contactAddr := types.NewAddress("", fromURI)
	reqBuilder.SetContact(contactAddr)
	
	// Строим запрос
	invite, err := reqBuilder.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build INVITE: %w", err)
	}

	// Применяем опции
	// Пока пропускаем, так как opts ожидает *Request, а у нас types.Message
	// TODO: адаптировать или изменить InviteOpts

	// Создаем UAC диалог
	dialogKey := DialogKey{
		CallID:    callID,
		LocalTag:  fromTag,
		RemoteTag: "", // Будет установлен после получения ответа
	}
	dialog := NewDialog(dialogKey, true, fromURI, target, s.txManager)

	// Создаем INVITE транзакцию
	tx, err := s.txManager.CreateClientTransaction(invite)
	if err != nil {
		return nil, fmt.Errorf("failed to create INVITE transaction: %w", err)
	}

	// Сохраняем транзакцию в диалоге
	dialog.inviteTx = tx

	// Обработчик ответов для INVITE
	tx.OnResponse(func(t transaction.Transaction, msg types.Message) {
		resp := msg.(*types.Response)
		s.handleInviteResponse(dialog, resp)
	})

	// Отправляем INVITE
	if err := tx.SendRequest(invite); err != nil {
		return nil, fmt.Errorf("failed to send INVITE: %w", err)
	}

	// Обновляем состояние диалога
	dialog.stateMachine.ProcessRequest(types.MethodINVITE, 0)

	// Сохраняем диалог
	if err := s.dialogManager.Add(dialog); err != nil {
		return nil, fmt.Errorf("failed to add dialog: %w", err)
	}

	return dialog, nil
}


// DialogByKey ищет существующий диалог
func (s *Stack) DialogByKey(key DialogKey) (IDialog, bool) {
	dialog, ok := s.dialogManager.Get(key)
	if !ok {
		return nil, false
	}
	return dialog, true
}

// OnIncomingDialog устанавливает обработчик для входящих диалогов
func (s *Stack) OnIncomingDialog(handler func(IDialog)) {
	s.handlersMutex.Lock()
	defer s.handlersMutex.Unlock()
	s.incomingDialogHandler = handler
}

// OnRequest устанавливает обработчик для запросов вне диалогов
func (s *Stack) OnRequest(method string, handler RequestHandler) {
	s.handlersMutex.Lock()
	defer s.handlersMutex.Unlock()
	s.requestHandlers[method] = handler
}

// handleIncomingRequest обрабатывает входящие запросы
func (s *Stack) handleIncomingRequest(tx transaction.Transaction, msg types.Message) {
	req := msg.(*types.Request)
	
	// Пытаемся найти существующий диалог
	key, err := GenerateDialogKey(req, false) // UAS role
	if err == nil && key.RemoteTag != "" {
		// Это in-dialog запрос
		dialog, ok := s.dialogManager.Get(key)
		
		if ok {
			// Передаем запрос диалогу
			if err := dialog.ProcessRequest(req); err != nil {
				// Отправляем ошибку
				respBuilder := builder.CreateResponse(req, 500, "Internal Server Error")
				resp, _ := respBuilder.Build()
				tx.SendResponse(resp)
			}
			return
		}
	}

	// Обрабатываем запросы вне диалога
	switch req.Method() {
	case types.MethodINVITE:
		s.handleIncomingInvite(tx, req)
	default:
		// Проверяем обработчики для метода
		s.handlersMutex.RLock()
		handler, ok := s.requestHandlers[req.Method()]
		s.handlersMutex.RUnlock()
		
		if ok {
			resp := handler(req)
			if resp != nil {
				tx.SendResponse(resp)
			}
		} else {
			// Метод не поддерживается
			respBuilder := builder.CreateResponse(req, 405, "Method Not Allowed")
			respBuilder.SetHeader("Allow", "INVITE, ACK, BYE, CANCEL, OPTIONS")
			resp, _ := respBuilder.Build()
			tx.SendResponse(resp)
		}
	}
}

// handleIncomingInvite обрабатывает входящий INVITE
func (s *Stack) handleIncomingInvite(tx transaction.Transaction, invite types.Message) {
	// Отправляем 100 Trying
	tryingBuilder := builder.CreateResponse(invite, 100, "Trying")
	trying, _ := tryingBuilder.Build()
	tx.SendResponse(trying)

	// Извлекаем информацию для диалога
	callID := invite.GetHeader("Call-ID")
	fromHeader := invite.GetHeader("From")
	fromTag := extractTag(fromHeader)
	
	// Генерируем To tag для UAS
	toTag := GenerateLocalTag()

	// Создаем UAS диалог
	dialogKey := DialogKey{
		CallID:    callID,
		LocalTag:  toTag,    // Для UAS local tag - это To tag
		RemoteTag: fromTag,  // Для UAS remote tag - это From tag
	}
	
	// Парсим URI из заголовков
	fromURI, _ := types.ParseURI(extractURIFromHeader(fromHeader))
	toURI, _ := types.ParseURI(extractURIFromHeader(invite.GetHeader("To")))
	
	// Для UAS: localURI = To, remoteURI = From
	dialog := NewDialog(dialogKey, false, toURI, fromURI, s.txManager)
	dialog.inviteTx = tx
	
	// Обновляем target из Contact запроса
	// Для UAS начальный target - это URI из Contact заголовка INVITE
	if contact := invite.GetHeader("Contact"); contact != "" {
		if contactURI, err := parseContactURI(contact); err == nil {
			dialog.targetManager.mu.Lock()
			dialog.targetManager.targetURI = contactURI
			dialog.targetManager.mu.Unlock()
		}
	}

	// Обновляем CSeq
	if cseqHeader := invite.GetHeader("CSeq"); cseqHeader != "" {
		if cseq, err := types.ParseCSeq(cseqHeader); err == nil {
			// Валидируем и сохраняем удаленный CSeq
			dialog.sequenceManager.ValidateRemoteCSeq(cseq.Sequence, cseq.Method)
			// Сохраняем CSeq от INVITE для ACK
			dialog.sequenceManager.SetInviteCSeq(cseq.Sequence, cseq.Method)
		}
	}

	// Обновляем состояние
	dialog.stateMachine.ProcessRequest(types.MethodINVITE, 0)

	// Сохраняем диалог
	if err := s.dialogManager.Add(dialog); err != nil {
		// Логируем ошибку
		fmt.Printf("Failed to add dialog: %v\n", err)
	}

	// Вызываем обработчик
	s.handlersMutex.RLock()
	handler := s.incomingDialogHandler
	s.handlersMutex.RUnlock()
	
	if handler != nil {
		handler(dialog)
	}
}

// handleIncomingResponse обрабатывает входящие ответы
func (s *Stack) handleIncomingResponse(tx transaction.Transaction, resp types.Message) {
	
	// Извлекаем CSeq для определения метода
	cseqHeader := resp.GetHeader("CSeq")
	cseq, err := types.ParseCSeq(cseqHeader)
	if err != nil {
		return
	}
	method := cseq.Method

	// Для ответов на INVITE обрабатываем особым образом
	if method == types.MethodINVITE {
		// Ответ должен обрабатываться через колбэк транзакции
		// который был установлен при создании INVITE
		return
	}

	// Для других методов пытаемся найти диалог
	key, err := GenerateDialogKey(resp, true) // UAC role для ответов
	if err != nil {
		return
	}

	dialog, ok := s.dialogManager.Get(key)
	
	if ok {
		// Передаем ответ диалогу
		dialog.ProcessResponse(resp, method)
	}
}

// handleInviteResponse обрабатывает ответы на INVITE
func (s *Stack) handleInviteResponse(dialog *Dialog, resp types.Message) {
	statusCode := resp.StatusCode()

	// Обновляем remote tag из To заголовка для UAC
	if dialog.isUAC && dialog.key.RemoteTag == "" {
		toHeader := resp.GetHeader("To")
		if toTag := extractTag(toHeader); toTag != "" {
			dialog.key.RemoteTag = toTag
			
			// Обновляем диалог в мапе с новым ключом
			oldKey := dialog.Key()
			dialog.key = DialogKey{
				CallID:    dialog.key.CallID,
				LocalTag:  dialog.key.LocalTag,
				RemoteTag: toTag,
			}
			if err := s.dialogManager.UpdateKey(oldKey, dialog.key); err != nil {
				// Логируем ошибку
				fmt.Printf("Failed to update dialog key: %v\n", err)
			}
		}
	}

	// Обновляем target из Contact
	dialog.targetManager.UpdateFromResponse(resp, types.MethodINVITE)

	// Обрабатываем в зависимости от кода
	switch {
	case statusCode >= 100 && statusCode < 200:
		// Provisional response
		dialog.stateMachine.ProcessResponse(types.MethodINVITE, statusCode)
		
	case statusCode >= 200 && statusCode < 300:
		// Success - отправляем ACK
		dialog.stateMachine.ProcessResponse(types.MethodINVITE, statusCode)
		
		// Создаем и отправляем ACK
		ack := dialog.createRequest(types.MethodACK)
		// ACK идет напрямую, не через транзакцию
		// Получаем target из диалога
		target := dialog.targetManager.GetTargetURI()
		if target != nil {
			targetAddr := fmt.Sprintf("%s:%d", target.Host(), target.Port())
			if err := s.transportManager.Send(ack, targetAddr); err != nil {
				fmt.Printf("Failed to send ACK: %v\n", err)
			}
		}
		
	case statusCode >= 300:
		// Failure
		dialog.stateMachine.ProcessResponse(types.MethodINVITE, statusCode)
		
		// Удаляем диалог
		s.dialogManager.Remove(dialog.Key())
	}
}

// extractURIFromHeader извлекает URI из заголовка From/To
func extractURIFromHeader(header string) string {
	// Простая реализация - ищем < и >
	start := -1
	end := -1
	
	for i, ch := range header {
		if ch == '<' {
			start = i + 1
		} else if ch == '>' && start != -1 {
			end = i
			break
		}
	}
	
	if start != -1 && end != -1 {
		return header[start:end]
	}
	
	// Если нет скобок, возвращаем всю строку до параметров
	if idx := strings.Index(header, ";"); idx != -1 {
		return strings.TrimSpace(header[:idx])
	}
	
	return strings.TrimSpace(header)
}

// GenerateCallID генерирует уникальный Call-ID
func GenerateCallID() string {
	return fmt.Sprintf("%d.%d@%s", 
		time.Now().UnixNano(), 
		rand.Int63(), 
		"localhost") // TODO: использовать реальный домен
}