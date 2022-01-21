package main

type Server struct {
	commandQueue CommandQueue
}

func (s *Server) Enqueue(cmd *Command, reply *EnqueuedCommand) error {
	enqueued, err := s.commandQueue.Enqueue(cmd)
	*reply = *enqueued
	return err
}

type CommandQueue interface {
	Enqueue(cmd *Command) (*EnqueuedCommand, error)
}
