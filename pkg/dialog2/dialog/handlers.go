package dialog

import (
	"context"
	"fmt"
	"github.com/emiago/sipgo/sip"
	"log/slog"
	"strconv"
	"strings"
)

var (
	CallIDDoesNotExist = "empty call id"
	CallDoesNotExist   = "transaction not found"
)

// если есть объект то вовзращает его
func handleInvite(req *sip.Request, tx sip.ServerTransaction) {

	{
		slog.Debug("handleInvite")
	}

	callID := req.CallID()
	if callID == nil {
		resp := sip.NewResponseFromRequest(req, sip.StatusBadRequest, CallIDDoesNotExist, nil)
		err := tx.Respond(resp)
		if err != nil {
			//todo log error
			// невозможно записать ответ на запрос с ошибкой
			slog.Error("")
		}
		return
	}
	tagTo := GetToTag(req)
	sessia, ok := sessionsMap.Get(*callID, tagTo)
	if tagTo != "" {
		if ok == true {
			sessia.handleInvite(req, tx)
		} else {
			resp := sip.NewResponseFromRequest(req, sip.StatusCallTransactionDoesNotExists, CallDoesNotExist, nil)
			err := tx.Respond(resp)
			if err != nil {
				//todo log error
				// невозможно записать ответ на запрос с ошибкой
				slog.Error("")
			}
			return
		}
	} else {
		_, is := sessionsMap.GetWithTX(GetBranchID(req))
		if is || ok {
			// loop detected
			resp := sip.NewResponseFromRequest(req, sip.StatusLoopDetected, "", nil)
			err := tx.Respond(resp)
			if err != nil {
				//todo log error
				// невозможно записать ответ на запрос с ошибкой
				slog.Error("")
			}
			return
		} else {
			sessionDialog := newUAS(req, tx)
			sessionsMap.Put(*callID, tagTo, GetBranchID(req), sessionDialog)
			lTX, err := newTX(context.Background(), sessionDialog, Incoming, tx, req)
			sessionDialog.setFirstIncomingTX(lTX)
			if err != nil {
				//todo дополнить ошибки
				slog.Error("error create newTX")
			}
		}
	}
}

func handleCancel(req *sip.Request, tx sip.ServerTransaction) {
	slog.Debug("handleCancel",
		slog.String("req", req.String()), slog.String("body", string(req.Body())))

	//todo поменять стейт на завершенный

	callID := req.CallID()
	if callID == nil {
		resp := sip.NewResponseFromRequest(req, sip.StatusBadRequest, CallIDDoesNotExist, nil)
		err := tx.Respond(resp)
		if err != nil {
			slog.Error("handle cancel", slog.Any("error", err))
		}
		return
	}

	tagTo := GetToTag(req)
	sess, ok := sessionsMap.Get(*callID, tagTo)
	if ok {
		_, err := newTX(context.Background(), sess, Incoming, tx, req)
		if err != nil {
			slog.Error("handle cancel. error creating dialog tx", slog.Any("error", err))
			return
		}
	} else {
		sess, is := sessionsMap.GetWithTX(GetBranchID(req))
		if is {
			_, err := newTX(context.Background(), sess, Incoming, tx, req)
			if err != nil {
				slog.Error("handle cancel. error creating dialog tx", slog.Any("error", err))
				return
			}
		}
	}

}

func handleBye(req *sip.Request, tx sip.ServerTransaction) {

}

func handlerBye(req *sip.Request, tx sip.ServerTransaction) {
	{
		//todo debug logger
		slog.Debug("handleBye",
			slog.String("request", req.String()),
			slog.String("body", string(req.Body())))
	}

	resp := sip.NewResponseFromRequest(req, sip.StatusOK, "", nil)

	if req.Via() != nil {
		viaHeader := req.Via()
		var (
			host string
			port uint16
		)
		host = viaHeader.Host
		if viaHeader.Params != nil {
			if received, ok := viaHeader.Params.Get("received"); ok && received != "" {
				host = received
			}
			if viaHeader.Port != 0 {
				port = uint16(viaHeader.Port)
			} else if rport, ok := viaHeader.Params.Get("rport"); ok && rport != "" {
				if p, err := strconv.Atoi(rport); err == nil {
					port = uint16(p)
				}
			} else {
				port = uint16(sip.DefaultPort(req.Transport()))
			}

		}

		builderDest := strings.Builder{}
		builderDest.WriteString(host)
		builderDest.WriteByte(':')
		builderDest.WriteString(strconv.Itoa(int(port)))

		resp.SetDestination(builderDest.String())
	}

	err := tx.Respond(resp)
	if err != nil {
		slog.Error("respond to Bye err", // todo
			slog.Any("error", err),
			slog.String("CallID", req.CallID().String()))
	}

	callID := req.CallID()
	if callID != nil {
		tag := GetToTag(req)
		if is, found := sessionsMap.Delete(*callID, tag, ""); found {
			fmt.Println(is)
			//err = is.TX().SetState(ReqAccepted, req, &tx)
			//err = is.processIncomingEvent(ReqAccepted1, req, &tx) // todo
			if err != nil {
				slog.Error("processing incoming request accept err", // todo
					slog.Any("error", err),
					slog.String("CallID", req.CallID().String()))
			}
		}
	}
}

// обработка ACK на ответ клиента на 200 OK
func handleACK(req *sip.Request, tx sip.ServerTransaction) {
	{
		//todo debug logger
		slog.Debug("handleAck",
			slog.String("request", req.String()),
			slog.String("body", string(req.Body())))
	}

	callID := req.CallID()
	if callID != nil {
		tagTo := GetToTag(req)
		sess, ok := sessionsMap.Get(*callID, tagTo)
		if ok {
			sess.getFirstIncomingTX().processAck(req)
		}
		return

	} else {
		resp := sip.NewResponseFromRequest(req, sip.StatusBadRequest, "call id is empty", nil)
		err := tx.Respond(resp)
		if err != nil {
			//todo log error
			slog.Error("handleAck", slog.Any("error", err))
		}
		return
	}
}

func handleUpdate(req *sip.Request, tx sip.ServerTransaction) {
	{
		//todo debug logger
		slog.Debug("handleUpdate",
			slog.String("req", req.String()),
			slog.String("body", string(req.Body())))
	}

	response := sip.NewResponseFromRequest(req, sip.StatusOK, "", nil)
	err := tx.Respond(response)
	if err != nil {
		slog.Error("respond to Update err", // todo
			slog.Any("error", err),
			slog.String("CallID", req.CallID().String()))
	}
}

func handleOptions(req *sip.Request, tx sip.ServerTransaction) {
	{
		//todo debug logger
		slog.Debug("handleOptions",
			slog.String("req", req.String()),
			slog.String("body", string(req.Body())))
	}

	response := sip.NewResponseFromRequest(req, sip.StatusOK, "", nil)
	err := tx.Respond(response)
	if err != nil {
		slog.Error("respond to Options err", // todo
			slog.Any("error", err),
			slog.String("CallID", req.CallID().String()))
	}
}

func handleNotify(req *sip.Request, tx sip.ServerTransaction) {
	{
		//todo debug logger
		slog.Debug("handleNotify",
			slog.String("req", req.String()),
			slog.String("body", string(req.Body())))
	}

	response := sip.NewResponseFromRequest(req, sip.StatusOK, "", nil)
	err := tx.Respond(response)
	if err != nil {
		slog.Error("respond to Notify err", // todo
			slog.Any("error", err),
			slog.String("CallID", req.CallID().String()))
	}
}

func handleRegister(req *sip.Request, tx sip.ServerTransaction) {
	err := tx.Respond(sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil))
	if err != nil {
		slog.Error("respond to Register", slog.Any("error", err))
	}
}
