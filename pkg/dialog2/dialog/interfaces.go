package dialog

import (
	"context"
	"github.com/emiago/sipgo/sip"
)

type IDialog interface {
	SetCallBacks(cb CallBacksDialog)
	// Invite Запрос sip
	Invite(ctx context.Context, target *sip.Uri, headers []sip.Header, body *Body) (ITx, error)
	Bye(ctx context.Context) error
}

type ITx interface {
	States() chan NotifyTX
	Answer200(body *Body) error
}

type CallBacks interface {
	OnIncomingCall(dialog IDialog, tx ITx)
}

type CallBacksDialog interface {
	OnChangeDialogState(state SessionState)
	OnNewTX(tx ITx)
}
