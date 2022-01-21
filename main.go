package main

import (
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
		_, err := app.TryEnqueue(cmd)
		if err == nil {
			log.Printf("enqueued '%s'\n", cmd)
			return
		}
	}

	app.Enqueue(cmd)
	app.Serve()
}

type Clock interface {
	Now() time.Time
}

type DefaultClock struct{}

func (DefaultClock) Now() time.Time {
	return time.Now()
}

type App struct {
	sockAddress string
	server      *Server
	cmdQueue    chan *Command
	done        chan bool
	executor    CommandExecutor
	clock       Clock
}

func (a *App) processQueue() {
	idleTimeout := make(<-chan time.Time)
	for {
		select {
		case <-idleTimeout:
			a.done <- true
			return
		case cmd := <-a.cmdQueue:
			a.executor.Execute(cmd)
			log.Println("idling...")
			idleTimeout = time.After(time.Second * 20)
		}
	}
}

func (a *App) Enqueue(cmd *Command) (*EnqueuedCommand, error) {
	go func() {
		log.Printf("enqueueing '%s'\n", cmd)
		a.cmdQueue <- cmd
	}()
	return &EnqueuedCommand{
		Command:    *cmd,
		EnqueuedAt: a.clock.Now(),
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

	stopSock := make(chan bool, 1)
	go a.serveRpc(stopSock)

	<-a.done

	log.Println("idle timeout reached, shutting down")
	close(a.cmdQueue)
	stopSock <- true
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

func (a *App) serveRpc(stopCh chan bool) {
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

	log.Println("listening on", a.sockAddress)

	go func() {
		<-stopCh
		sock.Close()
	}()

	http.Serve(sock, nil)
}

type CommandExecutor interface {
	Execute(cmd *Command)
}

type DefaultCommandExecutor struct {
}

func (e *DefaultCommandExecutor) Execute(cmd *Command) {
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

	log.Printf("started executing '%s' with pid %d\n", cmd, c.Process.Pid)

	err = c.Wait()
	exitCode := c.ProcessState.ExitCode()
	if err != nil && exitCode != 0 {
		log.Printf("execution failed. %s\n", c.ProcessState)
		return
	}

	log.Printf("finished: %s\n", c.ProcessState)
}

func NewApp(sockAddress string) *App {
	a := &App{
		sockAddress: sockAddress,
		cmdQueue:    make(chan *Command),
		done:        make(chan bool),
		executor:    &DefaultCommandExecutor{},
		clock:       &DefaultClock{},
	}
	a.server = &Server{commandQueue: a}
	return a
}
