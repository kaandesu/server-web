package main

import (
	"context"
	"encoding/gob"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type (
	Server struct {
		serverListener net.Listener
		userListener   net.Listener
		msgserverch    chan Message
		msgch          chan Message
		done           chan os.Signal
		Servers        map[string]*Server
		Clients        map[string]*Client
		Addr           string
		Apex           bool
		Registered     bool
		Capacity       int
	}
	Client struct {
		parentServer *Server
		con          net.Conn
		Addr         string
	}
	Message struct {
		fromServer *Server
		fromClient *Client
		con        net.Conn
		payload    []byte
	}
)

const (
	AuxAddr         = "127.0.0.1:"
	ApexAddr        = "127.0.0.1:3000"
	DefaultCapacity = 5
)

func NewServer(addr string, isApex bool) *Server {
	s := &Server{
		msgserverch: make(chan Message),
		msgch:       make(chan Message),
		done:        make(chan os.Signal),
		Servers:     make(map[string]*Server),
		Clients:     make(map[string]*Client),
		Registered:  isApex,
		Apex:        isApex,
		Addr:        addr,
		Capacity:    DefaultCapacity,
	}
	if !isApex {
		s.Servers[ApexAddr] = &Server{Addr: ApexAddr, Capacity: DefaultCapacity}
	}

	return s
}

func (s *Server) Start() {
	var err error
	s.serverListener, err = net.Listen("tcp", s.Addr)
	if err != nil {
		slog.Error("Could not start the server", "err", err)
	}

	go s.DialServer()
	go s.acceptServer()
	go s.handleUserMessages()
	go s.handleServerMessage()

	signal.Notify(s.done, os.Interrupt, syscall.SIGTERM, syscall.SIGABRT)
	<-s.done

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	if err = s.Shutdown(ctx); err != nil {
		slog.Error("Could not stop the server", "err", err)
	}
}

func (s *Server) DialServer() {
	if s.Apex {
		return
	}

	var (
		con net.Conn
		err error
	)

	for server := range s.Servers {
		con, err = net.Dial("tcp", server)
		if err == nil {
			break
		}
	}

	if con == nil {
		return
	}
	encoder := gob.NewEncoder(con)
	if err := encoder.Encode(*s); err != nil {
		slog.Error("Could not serialize the server", "err", err)
	}
	defer con.Close()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return nil
}

func (s *Server) acceptServer() {
	for {
		con, err := s.serverListener.Accept()
		if err != nil {
			break
		}
		go s.handleServerConnection(con)
	}
}

func (s *Server) handleServerConnection(con net.Conn) {
	node := NewServer(con.RemoteAddr().String(), false)
	s.Servers[node.Addr] = node
	buffer := make([]byte, 2048) // magic number?
	for {
		n, err := con.Read(buffer)
		if err != nil {
			slog.Info("Server disconnected from server", "err", err)
			break
		}

		s.msgserverch <- Message{
			fromServer: node,
			fromClient: nil,
			con:        con,
			payload:    buffer[:n],
		}

		// TODO: first message will contain info about:
		// - server list
		// - capacity
		// on this message, update the entry on this ones server list
		// Maybe protocol can have an "update" field, and we can handle
		// this stuff above while handling the messages???!!!!!!
		// flip the "registered" field once updated

	}
}

func (s *Server) handleUserMessages() {}

func (s *Server) handleServerMessage() {
	var receivedServer Server
	for msg := range s.msgserverch {
		decoder := gob.NewDecoder(msg.con)
		if err := decoder.Decode(receivedServer); err != nil {
			slog.Error("Could not deserialize the server", "err", err)
		}

		fmt.Println("Received:", receivedServer)

		// slog.Info("[server][msg]", "msg", string(msg.payload), "from", msg.fromServer.addr)
	}
}

func main() {
	apexFlag := flag.Bool("apex", false, "--apex | --apex=true | --apex=false [default false]")
	flag.Parse()

	var server *Server
	if *apexFlag {
		server = NewServer(ApexAddr, true)
	} else {
		server = NewServer(AuxAddr, false)
	}
	slog.Info("Starting the server on", "addr", server.Addr, "IsApex", server.Apex)
	server.Start()
}
