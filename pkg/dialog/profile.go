package dialog

import "github.com/emiago/sipgo/sip"

type Profile struct {
	DisplayName string
	Address     sip.Uri
}

// Contact создает хедер Contact
func (p *Profile) Contact() *sip.ContactHeader {
	contact := &sip.ContactHeader{
		DisplayName: p.DisplayName,
		Address:     p.Address,
	}
	return contact
}

func (p *Profile) Clone() *Profile {
	clone := &Profile{
		DisplayName: p.DisplayName,
		Address:     *p.Address.Clone(),
	}
	return clone
}
