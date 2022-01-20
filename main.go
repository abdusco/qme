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

const sockAddr = "/tmp/qme.sock"

type Command struct {
	WorkingDirectory string
	Command          string
	Args             []string
	Env              []string
}

func (c Command) String() string {
	return c.Command
}

type EnqueuedCommand struct {
	Command
	EnqueuedAt time.Time
}

type App struct {
	sockAddress string
	server      *Server
	cmdQueue    chan *Command
}

func (a App) processQueue() {
	for {
		select {
		case cmd := <-a.cmdQueue:
			log.Printf("worker: got job: %s", cmd)
			execute(cmd)
		case <-done:
			return
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

func (a App) TryEnqueue(cmd *Command) (*EnqueuedCommand, error) {
	client, err := rpc.DialHTTP("unix", sockAddr)
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

func NewApp(sockAddress string) *App {
	a := &App{
		sockAddress: sockAddress,
		cmdQueue:    make(chan *Command),
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

func main() {
	app := NewApp("/tmp/qme.sock")

	cmd, err := newCommand(os.Args, os.Environ())
	if err == nil {
		enqueued, err := app.TryEnqueue(cmd)
		if err == nil {
			fmt.Printf("enqueued %s at %s", cmd, enqueued.EnqueuedAt)
			return
		}
	}

	if err := os.RemoveAll(sockAddr); err != nil {
		log.Printf("Error removing old socket file: %s", err)
	}

	log.Println("assuming server role")
	sock, err := net.Listen("unix", sockAddr)
	if err != nil {
		log.Fatalf("failed to listen: %s\n", err)
	}

	err = rpc.Register(app.server)
	if err != nil {
		log.Fatal("Register error:", err)
	}
	rpc.HandleHTTP()

	fmt.Println("listening on", sockAddr)
	go http.Serve(sock, nil)

	app.processQueue()
}

func newCommand(args []string, env []string) (*Command, error) {
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

var done = make(chan bool)

func execute(cmd *Command) {
	log.Printf("executing: %+v\n", cmd.Command)

	c := exec.Command(cmd.Command, cmd.Args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Dir = cmd.WorkingDirectory
	c.Env = cmd.Env

	err := c.Start()
	if err != nil {
		log.Printf("failed to execute command %s: %s\n", cmd.Command, err)
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
