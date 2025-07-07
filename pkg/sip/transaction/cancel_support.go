package transaction

import (
	"fmt"

	"github.com/arzzra/soft_phone/pkg/sip/core/builder"
	"github.com/arzzra/soft_phone/pkg/sip/core/types"
)

// CancelSupport предоставляет поддержку CANCEL для транзакций
type CancelSupport struct {
	manager TransactionManager
	builder *MessageBuilder
}

// NewCancelSupport создает новый экземпляр поддержки CANCEL
func NewCancelSupport(manager TransactionManager) *CancelSupport {
	return &CancelSupport{
		manager: manager,
		builder: NewMessageBuilder(),
	}
}

// CancelTransaction отменяет указанную транзакцию
func (cs *CancelSupport) CancelTransaction(tx Transaction) error {
	// CANCEL можно отправить только для клиентской транзакции
	if !tx.IsClient() {
		return fmt.Errorf("can only cancel client transactions")
	}

	// CANCEL можно отправить только в состоянии Proceeding
	if tx.State() != TransactionProceeding {
		return fmt.Errorf("can only cancel transaction in Proceeding state, current: %s", tx.State())
	}

	// Получаем исходный запрос
	request := tx.Request()
	if request == nil {
		return fmt.Errorf("no request found in transaction")
	}

	// Не можем отменить ACK или CANCEL
	if request.Method() == "ACK" || request.Method() == "CANCEL" {
		return fmt.Errorf("cannot cancel %s request", request.Method())
	}

	// Создаем CANCEL запрос
	cancel, err := cs.builder.BuildCANCEL(request)
	if err != nil {
		return fmt.Errorf("failed to build CANCEL: %w", err)
	}

	// Создаем новую клиентскую транзакцию для CANCEL
	// CANCEL создает свою собственную транзакцию
	cancelTx, err := cs.manager.CreateClientTransaction(cancel)
	if err != nil {
		return fmt.Errorf("failed to create CANCEL transaction: %w", err)
	}

	// Регистрируем обработчик для отслеживания результата CANCEL
	cancelTx.OnResponse(func(t Transaction, resp types.Message) {
		statusCode := resp.StatusCode()
		if statusCode >= 200 && statusCode <= 299 {
			// CANCEL принят - исходная транзакция должна получить 487 Request Terminated
		}
	})

	return nil
}

// HandleCANCELRequest обрабатывает входящий CANCEL запрос
func (cs *CancelSupport) HandleCANCELRequest(cancel types.Message) error {
	if !cancel.IsRequest() || cancel.Method() != "CANCEL" {
		return fmt.Errorf("not a CANCEL request")
	}

	// Генерируем ключ для поиска исходной транзакции
	// CANCEL имеет тот же branch, но метод из исходного запроса
	via := cancel.GetHeader("Via")
	branch := extractBranch(via)
	
	// Получаем метод из CSeq (это метод CANCEL)
	// Но нам нужен метод исходного запроса
	// Обычно это INVITE, но может быть и другой
	
	// Создаем временный ключ для поиска
	// Будем искать все транзакции с этим branch
	searchKey := TransactionKey{
		Branch:    branch,
		Method:    "INVITE", // Обычно отменяют INVITE
		Direction: false,    // server transaction
	}

	// Ищем исходную транзакцию
	originalTx, found := cs.manager.FindTransaction(searchKey)
	if !found {
		// Попробуем найти другие методы
		// В реальной реализации нужно хранить маппинг CANCEL -> оригинальная транзакция
		return fmt.Errorf("matching transaction not found")
	}

	// Проверяем состояние
	state := originalTx.State()
	if state != TransactionProceeding {
		// Если транзакция уже завершена, отвечаем 481
		return fmt.Errorf("transaction in wrong state: %s", state)
	}

	// Отменяем исходную транзакцию
	// Это должно привести к отправке 487 Request Terminated
	// Реализация зависит от типа транзакции

	return nil
}

// CreateCANCELResponse создает ответ на CANCEL запрос
func (cs *CancelSupport) CreateCANCELResponse(cancel types.Message, statusCode int) (types.Message, error) {
	if !cancel.IsRequest() || cancel.Method() != "CANCEL" {
		return nil, fmt.Errorf("not a CANCEL request")
	}

	// Используем builder.CreateResponse для создания ответа
	respBuilder := builder.CreateResponse(cancel, statusCode, getReasonPhrase(statusCode))

	// Content-Length
	respBuilder.SetHeader("Content-Length", "0")

	return respBuilder.Build()
}

// getReasonPhrase возвращает фразу для кода состояния
func getReasonPhrase(code int) string {
	switch code {
	case 200:
		return "OK"
	case 481:
		return "Call/Transaction Does Not Exist"
	case 487:
		return "Request Terminated"
	default:
		return ""
	}
}