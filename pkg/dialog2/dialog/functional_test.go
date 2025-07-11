package dialog

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

type caseTest struct {
	inputReq   []string
	inputBody  []string
	expectResp []string
	expectBody []string
}

func initTestingDialog() *tools.MockedUDP {
	uu, err := NewUACUAS(Config{TestMode: true})
	if err != nil {
		panic(err)
	}
	tMocked := tools.NewMockedUDP()

	go func() {
		err := uu.ServeUDP(tMocked)
		if err != nil {
			panic(err)
		}
	}()
	return tMocked
}

func slice2Msg(msg, body []string) []byte {
	var data []byte
	for _, v := range msg {
		data = append(data, []byte(v)...)
		data = append(data, []byte("\r\n")...)
	}
	//if len(body) > 0 {
	//	data = append(data, []byte("\r\n")...)
	//}
	for _, v := range body {
		data = append(data, []byte("\r\n")...)
		data = append(data, []byte(v)...)
	}
	return data
}

func TestIncomingCall(t *testing.T) {

	cases := []caseTest{{
		inputReq: []string{
			`INVITE sip:822226024@tvlds-surec0019:5060 SIP/2.0`,
			`Via: SIP/2.0/UDP 10.34.200.230:5060;branch=z9hG4bKj8v79610fo6239ru0nl0.1`,
			`To: <sip:822226024@172.31.98.89>`,
			`From: "Иванов И.И." <sip:855778095@10.119.242.211>;tag=snl_d2nZ9ow43h`,
			`Call-ID: SEC11-80f8770a-81f8770a-1-ir6I9yi5VP3l`,
			`CSeq: 1235 INVITE`,
			`Contact: <sip:855778095@10.34.200.230:5060;transport=udp>`,
			`Content-Type: application/sdp`,
			`Content-Length: 526`,
			`Accept-Language: en;q=0.0`,
			`Supported: resource-priority`,
			`Date: Mon, 07 Oct 2024 05:31:46 GMT`,
			`Max-Forwards: 64`,
			`P-Asserted-Identity: "Иванов И.И." <sip:855778095@10.119.242.211>`,
			`Privacy: none`},
		inputBody: []string{
			`v=0`,
			`o=OpenStage-Line_0-0_mline 1338286374 1428626433 IN IP4 10.34.200.230`,
			`s=SIP Call`,
			`c=IN IP4 10.34.200.230`,
			`t=0 0`,
			`m=audio 38212 RTP/AVP 124 8 0 18 9 101`,
			`a=rtpmap:124 opus/48000/2`,
			`a=rtpmap:8 PCMA/8000`,
			`a=rtpmap:0 PCMU/8000`,
			`a=rtpmap:18 G729/8000`,
			`a=rtpmap:9 G722/8000`,
			`a=rtpmap:101 telephone-event/8000`,
			`a=silenceSupp:off - - - -`,
			`a=fmtp:124 maxaveragebitrate=16000;maxplaybackrate=16000;stereo=0;cbr=0;useinbandfec=0;usedtx=0;sprop-maxcapturerate=16000;sprop-stereo=0`,
			`a=fmtp:18 annexb=no`,
			`a=fmtp:101 0-15`,
			`a=sendrecv`},
		expectResp: []string{
			`SIP/2.0 100 Trying`,
			`Via: SIP/2.0/UDP 10.34.200.230:5060;branch=z9hG4bKj8v79610fo6239ru0nl0.1`,
			`From: "Иванов И.И." <sip:855778095@10.119.242.211>;tag=snl_d2nZ9ow43h`,
			`To: <sip:822226024@172.31.98.89>`,
			`Call-ID: SEC11-80f8770a-81f8770a-1-ir6I9yi5VP3l`,
			`CSeq: 1235 INVITE`,
			`Content-Length: 0`,
		},
		expectBody: []string{""},
	}, {expectResp: []string{
		`SIP/2.0 200 OK`,
		`Via: SIP/2.0/UDP 10.34.200.230:5060;branch=z9hG4bKj8v79610fo6239ru0nl0.1`,
		`From: "Иванов И.И." <sip:855778095@10.119.242.211>;tag=snl_d2nZ9ow43h`,
		`To: <sip:822226024@172.31.98.89>;tag=LguzO09S`,
		`Call-ID: SEC11-80f8770a-81f8770a-1-ir6I9yi5VP3l`,
		`CSeq: 1235 INVITE`,
		`Content-Length: 227`,
		`Contact: <sip:822226024@tvlds-surec0019:5060>`,
		`Content-Type: application/sdp`,
	},
	},
	}

	mockConn := initTestingDialog()

	for _, v := range cases {
		mockConn.SendToUDP(slice2Msg(v.inputReq, v.inputBody))
		resp := mockConn.RecvFromUDP()
		assert.Equal(t, string(slice2Msg(v.expectResp, v.expectBody)), string(resp))
		// отвечаем 200 ок

	}

}

func TestOutgoingCall(t *testing.T) {

}
