package dialog

import "github.com/emiago/sipgo/sip"

// Profile представляет профиль пользователя SIP.
// Используется для идентификации в SIP сообщениях и установления контакта.
type Profile struct {
	// DisplayName - отображаемое имя пользователя (например, "Alice Smith")
	DisplayName string
	// Address - SIP адрес пользователя (например, sip:alice@example.com)
	Address sip.Uri
}

// Contact создает заголовок Contact на основе профиля.
// Contact содержит адрес, по которому можно связаться с пользователем.
func (p *Profile) Contact() *sip.ContactHeader {
	contact := &sip.ContactHeader{
		DisplayName: p.DisplayName,
		Address:     p.Address,
	}
	return contact
}

// Clone создает глубокую копию профиля.
// Используется для создания независимых копий при создании новых диалогов.
func (p *Profile) Clone() *Profile {
	clone := &Profile{
		DisplayName: p.DisplayName,
		Address:     *p.Address.Clone(),
	}
	return clone
}
