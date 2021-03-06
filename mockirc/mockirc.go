package mockirc

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"time"
)

type MockIRC struct {
	Port       int
	ServerName string
	Socket     net.Listener
	events     map[string]*WhenEvent
	io         *bufio.ReadWriter
	stop       bool
}

// New will create a new instance of mockirc.
// Returns a pointer to a MockIRC struct.
func New(server string, port int) *MockIRC {
	irc := &MockIRC{}
	if port == 0 {
		irc.Port = 6661
	} else {
		irc.Port = port
	}
	irc.ServerName = server
	irc.events = make(map[string]*WhenEvent)
	return irc
}

// Start will start the "irc" server and listen on the port passed to New.
// Returns a channel of type bool or an error.
func (irc *MockIRC) Start() (chan bool, error) {
	wait := make(chan bool, 1)
	sock, err := net.Listen("tcp", fmt.Sprintf(":%d", irc.Port))
	if err != nil {
		return nil, err
	}
	irc.Socket = sock
	go func() {
		defer close(wait)

		accept := func() <-chan net.Conn {
			a := make(chan net.Conn)
			go func() {
				for {
					conn, err := irc.Socket.Accept()
					if err != nil {
						if irc.stop {
							return
						}
						log.Println(err)
						continue
					}
					if conn != nil {
						a <- conn
					}
				}
			}()
			return a
		}()

		wait <- true

		for {
			conn := <-accept
			if irc.stop {
				return
			}
			go irc.connectionWorker(conn)
		}
	}()
	return wait, nil
}

// Stop will send the shutdown message on the control channel and stop the server.
// It could return an error.
func (irc *MockIRC) Stop() error {
	if irc.stop {
		return nil
	}

	irc.stop = true
	err := irc.Socket.Close()
	if err != nil {
		return err
	}
	<-time.After(time.Second)
	return nil
}

// connectionWorker will handle incoming connections from Accept.
// Runs in it's own goroutine.
func (irc *MockIRC) connectionWorker(conn net.Conn) {
	irc.io = bufio.NewReadWriter(
		bufio.NewReader(conn),
		bufio.NewWriter(conn))
	defer conn.Close()
	for {
		msg, err := irc.io.ReadString('\n')
		if err != nil {
			if !irc.stop {
				if err == io.EOF {
					log.Println(err)
				}
			}
			return
		}
		irc.handleMessage(msg)
	}
}

// handleMessage will figure out how to handle messages coming in. It looks at the
// events map to see if it matched anything to send a response.
func (irc *MockIRC) handleMessage(msg string) {
	if !irc.stop {
		msg = strings.TrimSuffix(msg, "\r\n")
		var err error
		if val, ok := irc.events[msg]; ok {
			for _, response := range val.responses {
				_, err = irc.io.WriteString(response + "\r\n")
				if err != nil {
					log.Println(err)
					return
				}
				err = irc.io.Flush()
				if err != nil {
					log.Println(err)
					return
				}
			}
		} else {
			log.Printf("Nothing to do for %s\n", msg)
		}
		log.Println(msg)
	}
}

// Send will write the string to the connection.
func (irc *MockIRC) Send(thing string) {
	if !irc.stop {
		irc.io.WriteString(thing + "\r\n")
	}
}

type WhenEvent struct {
	event     string
	responses []string
}

// When will take a string that represents an event. This stores the event in a map
// that is checked later when a message comes in over a connection.
// Example use: `mockircserver.When("PING mockirc").Respond(":PONG mockirc")
// Returns the new WhenEvent instance for method chaining.
func (irc *MockIRC) When(event string) *WhenEvent {
	when := &WhenEvent{event: event}
	irc.events[event] = when
	return when
}

// This will add to a list of reponses to send back when an event is matched.
// Returns the new WhenEvent instance for method chaining.
func (when *WhenEvent) Respond(response string) *WhenEvent {
	when.responses = append(when.responses, response)
	return when
}
