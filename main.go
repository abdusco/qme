package main

import (
	"fmt"
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
}

var cmdQueue = make(chan *Command)

func (a *App) Enqueue(cmd *Command, reply *EnqueuedCommand) error {
	go func() {
		cmdQueue <- cmd
	}()
	reply.Command = *cmd
	reply.EnqueuedAt = time.Now()
	return nil
}

func main() {
	client, err := rpc.DialHTTP("unix", sockAddr)
	if err == nil {
		log.Println("connected. assuming client role")
		defer client.Close()
		var reply *EnqueuedCommand
		cwd, _ := os.Getwd()
		if len(os.Args) < 3 {
			log.Fatalln("usage: qme <command> <args>")
		}

		cmd, args := os.Args[1], os.Args[2:]
		item := &Command{
			WorkingDirectory: cwd,
			Command:          cmd,
			Args:             args,
			Env:              os.Environ(),
		}
		err = client.Call("App.Enqueue", item, &reply)
		if err != nil {
			log.Fatal("App.Enqueue error:", err)
		}
		log.Printf("enqueued %s %s at %s", cmd, args, reply.EnqueuedAt)
		return
	}

	if err := os.RemoveAll(sockAddr); err != nil {
		log.Printf("Error removing old socket file: %s", err)
	}

	log.Println("assuming server role")
	sock, err := net.Listen("unix", sockAddr)
	if err != nil {
		log.Fatalf("failed to listen: %s\n", err)
	}

	err = rpc.Register(new(App))
	if err != nil {
		log.Fatal("Register error:", err)
	}
	rpc.HandleHTTP()

	go worker()

	fmt.Println("listening on", sockAddr)
	log.Fatalln(http.Serve(sock, nil))
}

func worker() {
	for {
		select {
		case cmd := <-cmdQueue:
			log.Printf("worker: got job: %s", cmd)
			execute(cmd)
		}
	}
}

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
