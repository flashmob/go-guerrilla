package backends

import (
	"fmt"
	"github.com/flashmob/go-guerrilla/log"
	"github.com/flashmob/go-guerrilla/mail"
	"strings"
	"testing"
	"time"
)

func TestStates(t *testing.T) {
	gw := BackendGateway{}
	str := fmt.Sprintf("%s", gw.State)
	if strings.Index(str, "NewState") != 0 {
		t.Error("Backend should begin in NewState")
	}
}

func TestInitialize(t *testing.T) {
	c := BackendConfig{
		"save_process":       "HeadersParser|Debugger",
		"log_received_mails": true,
		"save_workers_size":  "1",
	}

	gateway := &BackendGateway{}
	err := gateway.Initialize(c)
	if err != nil {
		t.Error("Gateway did not init because:", err)
		t.Fail()
	}
	if gateway.processors == nil {
		t.Error("gateway.chains should not be nil")
	} else if len(gateway.processors) != 1 {
		t.Error("len(gateway.chains) should be 1, but got", len(gateway.processors))
	}

	if gateway.conveyor == nil {
		t.Error("gateway.conveyor should not be nil")
	} else if cap(gateway.conveyor) != gateway.workersSize() {
		t.Error("gateway.conveyor channel buffer cap does not match worker size, cap was", cap(gateway.conveyor))
	}

	if gateway.State != BackendStateInitialized {
		t.Error("gateway.State is not in initialized state, got ", gateway.State)
	}

}

func TestStartProcessStop(t *testing.T) {
	c := BackendConfig{
		"save_process":       "HeadersParser|Debugger",
		"log_received_mails": true,
		"save_workers_size":  2,
	}

	gateway := &BackendGateway{}
	err := gateway.Initialize(c)

	mainlog, _ := log.GetLogger(log.OutputOff.String(), "debug")
	Svc.SetMainlog(mainlog)

	if err != nil {
		t.Error("Gateway did not init because:", err)
		t.Fail()
	}
	err = gateway.Start()
	if err != nil {
		t.Error("Gateway did not start because:", err)
		t.Fail()
	}
	if gateway.State != BackendStateRunning {
		t.Error("gateway.State is not in rinning state, got ", gateway.State)
	}
	// can we place an envelope on the conveyor channel?

	e := &mail.Envelope{
		RemoteIP: "127.0.0.1",
		QueuedId: "abc12345",
		Helo:     "helo.example.com",
		MailFrom: mail.Address{User: "test", Host: "example.com"},
		TLS:      true,
	}
	e.PushRcpt(mail.Address{User: "test", Host: "example.com"})
	e.Data.WriteString("Subject:Test\n\nThis is a test.")
	notify := make(chan *notifyMsg)

	gateway.conveyor <- &workerMsg{e, notify, TaskSaveMail}

	// it should not produce any errors
	// headers (subject) should be parsed.

	select {
	case status := <-notify:

		if status.err != nil {
			t.Error("envelope processing failed with:", status.err)
		}
		if e.Header["Subject"][0] != "Test" {
			t.Error("envelope processing did not parse header")
		}

	case <-time.After(time.Second):
		t.Error("gateway did not respond after 1 second")
		t.Fail()
	}

	err = gateway.Shutdown()
	if err != nil {
		t.Error("Gateway did not shutdown")
	}
}
