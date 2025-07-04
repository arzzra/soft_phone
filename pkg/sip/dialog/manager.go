package dialog

import (
	"fmt"
	"sync"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/message"
	"github.com/arzzra/soft_phone/pkg/sip/transaction"
)

// manager implements the Manager interface
type manager struct {
	// Dialog storage
	dialogs sync.Map // key -> Dialog

	// Transaction manager
	txManager transaction.Manager

	// Cleanup
	cleanupTicker *time.Ticker
	done          chan struct{}
}

// NewManager creates a new dialog manager
func NewManager(txManager transaction.Manager) Manager {
	m := &manager{
		txManager:     txManager,
		cleanupTicker: time.NewTicker(30 * time.Second),
		done:          make(chan struct{}),
	}

	// Start cleanup routine
	go m.cleanupRoutine()

	return m
}

// CreateDialog creates a new dialog from initial INVITE
func (m *manager) CreateDialog(invite *message.Request, role Role) (Dialog, error) {
	if invite == nil || invite.Method != "INVITE" {
		return nil, ErrInvalidRequest
	}

	// Create dialog
	d, err := NewDialog(invite, role, m.txManager)
	if err != nil {
		return nil, err
	}

	// Generate key
	key := m.generateKey(d.CallID(), d.LocalTag(), d.RemoteTag())

	// Store dialog
	if _, loaded := m.dialogs.LoadOrStore(key, d); loaded {
		return nil, ErrDialogExists
	}

	// Register cleanup when dialog terminates
	d.OnStateChange(func(state State) {
		if state == StateTerminated {
			// Update key in case remote tag changed
			newKey := m.generateKey(d.CallID(), d.LocalTag(), d.RemoteTag())
			m.dialogs.Delete(newKey)
		}
	})

	return d, nil
}

// FindDialog finds existing dialog
func (m *manager) FindDialog(callID, localTag, remoteTag string) (Dialog, bool) {
	key := m.generateKey(callID, localTag, remoteTag)
	if d, ok := m.dialogs.Load(key); ok {
		return d.(Dialog), true
	}
	return nil, false
}

// HandleRequest routes request to appropriate dialog
func (m *manager) HandleRequest(req *message.Request) error {
	// Extract dialog identification
	callID := req.GetHeader("Call-ID")
	fromHeader := req.GetHeader("From")
	toHeader := req.GetHeader("To")

	fromTag := message.ExtractTag(fromHeader)
	toTag := message.ExtractTag(toHeader)

	// For requests from remote party:
	// LocalTag = To tag, RemoteTag = From tag
	if d, ok := m.FindDialog(callID, toTag, fromTag); ok {
		dialog := d.(*dialog)
		return dialog.ProcessRequest(req)
	}

	// New dialog for INVITE
	if req.Method == "INVITE" {
		// Create new UAS dialog
		d, err := m.CreateDialog(req, RoleUAS)
		if err != nil {
			return err
		}

		// Process the INVITE
		dialog := d.(*dialog)
		return dialog.ProcessRequest(req)
	}

	// Request for unknown dialog
	return ErrDialogNotFound
}

// HandleResponse routes response to appropriate dialog
func (m *manager) HandleResponse(resp *message.Response) error {
	// Extract dialog identification
	callID := resp.GetHeader("Call-ID")
	fromHeader := resp.GetHeader("From")
	toHeader := resp.GetHeader("To")

	fromTag := message.ExtractTag(fromHeader)
	toTag := message.ExtractTag(toHeader)

	// For responses to our requests:
	// LocalTag = From tag, RemoteTag = To tag

	// Try with full dialog ID first
	if d, ok := m.FindDialog(callID, fromTag, toTag); ok {
		dialog := d.(*dialog)
		return dialog.ProcessResponse(resp)
	}

	// For initial responses, remote tag might not be set yet in the dialog
	// Search for dialog with matching Call-ID and local tag
	var foundDialog *dialog
	m.dialogs.Range(func(key, value interface{}) bool {
		d := value.(*dialog)
		if d.CallID() == callID && d.LocalTag() == fromTag {
			// If dialog has no remote tag yet, or it matches
			if d.RemoteTag() == "" || d.RemoteTag() == toTag {
				foundDialog = d
				return false
			}
		}
		return true
	})

	if foundDialog != nil {
		oldRemoteTag := foundDialog.RemoteTag()

		// Process response (which may update remote tag)
		err := foundDialog.ProcessResponse(resp)

		// If remote tag was updated, re-key the dialog
		if oldRemoteTag == "" && foundDialog.RemoteTag() != "" {
			oldKey := m.generateKey(callID, fromTag, "")
			newKey := m.generateKey(callID, fromTag, foundDialog.RemoteTag())
			m.dialogs.Delete(oldKey)
			m.dialogs.Store(newKey, foundDialog)
		}

		return err
	}

	return ErrDialogNotFound
}

// Dialogs returns all active dialogs
func (m *manager) Dialogs() []Dialog {
	var dialogs []Dialog
	m.dialogs.Range(func(key, value interface{}) bool {
		dialogs = append(dialogs, value.(Dialog))
		return true
	})
	return dialogs
}

// Close closes the manager
func (m *manager) Close() error {
	// Stop cleanup routine
	close(m.done)
	m.cleanupTicker.Stop()

	// Terminate all dialogs
	m.dialogs.Range(func(key, value interface{}) bool {
		if d, ok := value.(Dialog); ok {
			d.Terminate()
		}
		return true
	})

	return nil
}

// generateKey generates dialog storage key
func (m *manager) generateKey(callID, localTag, remoteTag string) string {
	return fmt.Sprintf("%s:%s:%s", callID, localTag, remoteTag)
}

// cleanupRoutine periodically cleans up terminated dialogs
func (m *manager) cleanupRoutine() {
	for {
		select {
		case <-m.cleanupTicker.C:
			m.cleanupDialogs()
		case <-m.done:
			return
		}
	}
}

// cleanupDialogs removes terminated dialogs
func (m *manager) cleanupDialogs() {
	m.dialogs.Range(func(key, value interface{}) bool {
		if d, ok := value.(Dialog); ok {
			if d.State() == StateTerminated {
				m.dialogs.Delete(key)
			}
		}
		return true
	})
}
