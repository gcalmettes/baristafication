package baristafication

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"time"

	"barista.run/bar"
	"barista.run/base/value"
	"barista.run/outputs"
	"barista.run/timing"
)

// Rofication unix socket path
// https://github.com/regolith-linux/regolith-rofication/blob/master/rofication/_static.py#L8
const ROFICATION_UNIX_SOCK = "/tmp/rofi_notification_daemon"

// RoficationNotification data model.
// https://github.com/regolith-linux/regolith-rofication/blob/master/rofication/_notification.py#L18
type RoficationNotification struct {
	Id          int      `json:"id"`
	Summary     string   `json:"summary"`
	Body        string   `json:"body"`
	Application string   `json:"application"`
	Urgency     int      `json:"urgency"`
	Actions     []string `json:"actions"`
}

// Notifications represents the notifications grouped by applications. The key is the
// application, and the value is the number of notifications
// for that application.
type Notifications map[string]int

// Total returns the total number of unread notifications across all categories.
func (n Notifications) Total() int {
	t := 0
	for _, c := range n {
		t += c
	}
	return t
}

// Module represents a regolith-rofication bar module.
// It supports setting the output format and update frequency.
type Module struct {
	outputFunc value.Value // of func(Notifications) bar.Output

	// Control when we next check for notifications.
	scheduler *timing.Scheduler
	interval  int
}

// New creates a Baristafication module.
func New() *Module {
	m := &Module{
		scheduler: timing.NewScheduler(),
		// FIXME We want to make this time interval customizable
		interval: 10,
	}
	m.Output(func(n Notifications) bar.Output {
		if n.Total() == 0 {
			return nil
		}
		return outputs.Textf("G5: %d", n.Total())
	})
	return m
}

// This is a terrible hack.
var errCached = errors.New("NothingChanged")

// Stream starts the module.
func (m *Module) Stream(sink bar.Sink) {

	outf := m.outputFunc.Get().(func(Notifications) bar.Output)
	nextOutputFunc, done := m.outputFunc.Subscribe()
	defer done()
	info, err := m.getNotifications()

	for {
		if err != errCached {
			if sink.Error(err) {
				return
			}
			sink.Output(outf(info))
		}
		err = nil
		select {
		case <-nextOutputFunc:
			outf = m.outputFunc.Get().(func(Notifications) bar.Output)
		case <-m.scheduler.C:
			i, e := m.getNotifications()
			err = e
			if e != errCached {
				info = i
			}
		}
	}
}

func (m *Module) getNotifications() (Notifications, error) {
	conn, err := net.Dial("unix", ROFICATION_UNIX_SOCK)
	if err != nil {
		log.Fatal("Could not connect to rorification socket", err)
	}

	defer conn.Close()
	msg := "list\n"
	_, err = conn.Write([]byte(msg))
	if err != nil {
		log.Fatal("Could not send message to rorification socket", err)
	}

	m.scheduler.After(time.Duration(m.interval) * time.Second)

	var resp []RoficationNotification
	err = json.NewDecoder(conn).Decode(&resp)
	if err != nil {
		return nil, err
	}

	info := Notifications{}
	// fmt.Println(resp)
	for _, n := range resp {
		count := info[n.Application]
		count++
		info[n.Application] = count
	}
	// fmt.Println(info)
	return info, nil
}

// Output sets the output format for this module.
func (m *Module) Output(outputFunc func(Notifications) bar.Output) *Module {
	m.outputFunc.Set(outputFunc)
	return m
}
