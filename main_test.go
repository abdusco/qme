package main

import (
	"reflect"
	"testing"
	"time"
)

type BrokenClock struct {
	now time.Time
}

func (c *BrokenClock) Now() time.Time {
	return c.now
}

func TestApp_Enqueue(t *testing.T) {
	clock := &BrokenClock{now: time.Now()}

	type args struct {
		cmd *Command
	}
	tests := []struct {
		name    string
		args    args
		want    *EnqueuedCommand
		wantErr bool
	}{
		{
			name: "pushes command to queue",
			args: args{&Command{Command: "echo", Args: []string{"hello"}, Env: []string{"FOO=BAR"}, WorkingDirectory: "/cwd"}},
			want: &EnqueuedCommand{
				Command:    Command{Command: "echo", Args: []string{"hello"}, Env: []string{"FOO=BAR"}, WorkingDirectory: "/cwd"},
				EnqueuedAt: clock.now,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cmdQueue = make(chan *Command)
			a := &App{
				cmdQueue: cmdQueue,
				clock:    clock,
			}
			got, err := a.Enqueue(tt.args.cmd)
			if (err != nil) != tt.wantErr {
				t.Errorf("Enqueue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			select {
			case cmd := <-cmdQueue:
				if !reflect.DeepEqual(cmd, tt.args.cmd) {
					t.Errorf("Enqueue() got = %v, want %v", cmd, tt.args.cmd)
				}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Enqueue() got = %v, want %v", got, tt.want)
			}
		})
	}
}
