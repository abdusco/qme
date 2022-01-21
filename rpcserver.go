package main

import (
	"github.com/pkg/errors"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
)

type CommandQueue interface {
	Enqueue(cmd *Command) (*EnqueuedCommand, error)
}

type Server struct {
	commandQueue CommandQueue
	sockAddress  string
}

func (s *Server) Enqueue(cmd *Command, reply *EnqueuedCommand) error {
	enqueued, err := s.commandQueue.Enqueue(cmd)
	*reply = *enqueued
	return err
}

func (s *Server) sendCommand(cmd *Command) (*EnqueuedCommand, error) {
	client, err := rpc.DialHTTP("unix", s.sockAddress)
	if err != nil {
		return nil, err
	}
	log.Println("connected. assuming client role")
	defer client.Close()

	var reply *EnqueuedCommand
	err = client.Call("Server.Enqueue", cmd, &reply)
	if err != nil {
		return nil, errors.Wrap(err, "send command to server")
	}

	return reply, nil
}

func (s *Server) serve(stopCh <-chan bool) error {
	err := rpc.Register(s)
	if err != nil {
		log.Fatal("failed to set up rpc:", err)
	}
	rpc.HandleHTTP()

	if err := os.RemoveAll(s.sockAddress); err != nil {
		log.Printf("failed to remove old socket file: %s\n", err)
	}

	log.Println("assuming server role")
	sock, err := net.Listen("unix", s.sockAddress)
	if err != nil {
		log.Fatalf("failed to listen: %s\n", err)
	}

	log.Println("listening on", s.sockAddress)

	go func() {
		<-stopCh
		sock.Close()
	}()

	return http.Serve(sock, nil)
}
