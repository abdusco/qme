package main

import "time"

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
