package main

import (
	"fmt"
	"github.com/pkg/errors"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"os/exec"
	"time"
)

func main() {
	app := NewApp("/tmp/qme.sock")

	cmd, err := app.ParseCommand(os.Args, os.Environ())
	if err == nil {
		enqueued, err := app.TryEnqueue(cmd)
		if err == nil {
			fmt.Printf("enqueued %s at %s\n", cmd, enqueued.EnqueuedAt)
			return
		}
	}

	app.Enqueue(cmd)
	app.Serve()
}

type App struct {
	sockAddress string
	server      *Server
	cmdQueue    chan *Command
	done        chan bool
}

func (a *App) processQueue() {
	idleTimeout := make(<-chan time.Time)
	for {
		select {
		case <-idleTimeout:
			a.done <- true
			return
		case cmd := <-a.cmdQueue:
			log.Printf("worker: got job: %s", cmd)
			a.Execute(cmd)
			log.Println("worker: done, setting up timer")
			idleTimeout = time.After(time.Second * 20)
		}
	}
}

func (a *App) Enqueue(cmd *Command) (*EnqueuedCommand, error) {
	go func() {
		a.cmdQueue <- cmd
	}()
	return &EnqueuedCommand{
		Command:    *cmd,
		EnqueuedAt: time.Now(),
	}, nil
}

func (a *App) TryEnqueue(cmd *Command) (*EnqueuedCommand, error) {
	client, err := rpc.DialHTTP("unix", a.sockAddress)
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

func (a *App) Serve() {
	go a.processQueue()

	if err := os.RemoveAll(a.sockAddress); err != nil {
		log.Printf("failed to remove old socket file: %s\n", err)
	}

	log.Println("assuming server role")
	sock, err := net.Listen("unix", a.sockAddress)
	if err != nil {
		log.Fatalf("failed to listen: %s\n", err)
	}

	err = rpc.Register(a.server)
	if err != nil {
		log.Fatal("failed to set up rpc:", err)
	}
	rpc.HandleHTTP()

	fmt.Println("listening on", a.sockAddress)
	go http.Serve(sock, nil)

	<-a.done
	log.Println("idle timeout reached, shutting down")
	close(a.cmdQueue)
	sock.Close()
}

func (a *App) ParseCommand(args []string, env []string) (*Command, error) {
	if len(args) < 2 {
		return nil, errors.New("usage: qme <command> <args>")
	}

	cwd, _ := os.Getwd()
	cmd, args := args[1], args[2:]
	return &Command{
		WorkingDirectory: cwd,
		Command:          cmd,
		Args:             args,
		Env:              env,
	}, nil
}

func (a *App) Execute(cmd *Command) {
	log.Printf("executing: %+v\n", cmd.Command)

	c := exec.Command(cmd.Command, cmd.Args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Dir = cmd.WorkingDirectory
	c.Env = cmd.Env

	err := c.Start()
	if err != nil {
		log.Printf("failed to Execute command %s: %s\n", cmd.Command, err)
		return
	}

	log.Println("started executing command with pid", c.Process.Pid)

	err = c.Wait()
	exitCode := c.ProcessState.ExitCode()
	if err != nil && exitCode != 0 {
		log.Printf("command failed. %s\n", c.ProcessState)
		return
	}

	log.Printf("command finished: %s\n", c.ProcessState)
}

func NewApp(sockAddress string) *App {
	a := &App{
		sockAddress: sockAddress,
		cmdQueue:    make(chan *Command),
		done:        make(chan bool),
	}
	a.server = &Server{app: a}
	return a
}

type Server struct {
	app *App
}

func (s *Server) Enqueue(cmd *Command, reply *EnqueuedCommand) error {
	enqueued, err := s.app.Enqueue(cmd)
	*reply = *enqueued
	return err
}
