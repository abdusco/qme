package main

import (
	"fmt"
	"github.com/pkg/errors"
	"log"
	"os/signal"
	"syscall"

	//"net/rpc"
	"os"
	"os/exec"
	"time"
)

func main() {
	app := NewApp("/tmp/qme.sock")
	app.Run()
}

type Clock interface {
	Now() time.Time
}

type defaultClock struct{}

func (defaultClock) Now() time.Time {
	return time.Now()
}

type App struct {
	rpc         *Server
	cmdQueue    chan *Command
	quitCh      chan bool
	executor    CommandExecutor
	clock       Clock
	idleTimeout time.Duration
}

func (a *App) processQueue(killCh <-chan bool) {
	timeout := make(<-chan time.Time)
	for {
		select {
		case <-timeout:
			a.quitCh <- true
			return
		case cmd := <-a.cmdQueue:
			a.executor.Execute(cmd, killCh)
			log.Println("idling...")
			timeout = time.After(a.idleTimeout)
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

func (a *App) SendCommand(cmd *Command) (*EnqueuedCommand, error) {
	return a.rpc.sendCommand(cmd)
}

func (a *App) Serve() {
	killCh := make(chan bool)
	go a.processQueue(killCh)

	stopSockCh := make(chan bool, 1)
	go a.rpc.serve(stopSockCh)

	<-a.quitCh

	log.Println("shutting down")
	killCh <- true
	stopSockCh <- true
	close(a.cmdQueue)
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

func (a *App) Run() {
	cmd, err := a.ParseCommand(os.Args, os.Environ())
	if err == nil {
		_, err := a.SendCommand(cmd)
		if err == nil {
			log.Printf("enqueued '%s'\n", cmd)
			return
		}
	} else {
		fmt.Println(err)
		return
	}

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-signalCh
		log.Printf("%s signal received\n", sig)
		a.quitCh <- true
	}()

	a.Enqueue(cmd)
	a.Serve()
}

type CommandExecutor interface {
	Execute(cmd *Command, killCh <-chan bool)
}

type defaultCommandExecutor struct {
}

func (e *defaultCommandExecutor) Execute(cmd *Command, killCh <-chan bool) {
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

	go func() {
		<-killCh
		log.Printf("killing '%s'\n", cmd)
		c.Process.Kill()
	}()

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
		cmdQueue:    make(chan *Command),
		quitCh:      make(chan bool),
		executor:    &defaultCommandExecutor{},
		clock:       &defaultClock{},
		idleTimeout: time.Second * 20,
	}
	a.rpc = &Server{commandQueue: a, sockAddress: sockAddress}
	return a
}
