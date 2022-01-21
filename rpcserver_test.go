package main

import (
	"reflect"
	"testing"
	"time"
)

type fakeQueue struct {
	queued []*Command
	now    time.Time
}

func (q *fakeQueue) Enqueue(cmd *Command) (*EnqueuedCommand, error) {
	q.queued = append(q.queued, cmd)
	return &EnqueuedCommand{Command: *cmd, EnqueuedAt: q.now}, nil
}

func TestServer_Enqueue(t *testing.T) {
	type args struct {
		cmd   *Command
		reply *EnqueuedCommand
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "enqueues command",
			args: args{
				cmd: &Command{
					WorkingDirectory: "/cwd",
					Command:          "sleep",
					Args:             []string{"1"},
					Env:              []string{"FOO=BAR"},
				},
				reply: &EnqueuedCommand{},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fq := &fakeQueue{
				queued: []*Command{},
				now:    time.Now(),
			}
			s := &Server{
				commandQueue: fq,
			}
			if err := s.Enqueue(tt.args.cmd, tt.args.reply); (err != nil) != tt.wantErr {
				t.Errorf("Enqueue() error = %v, wantErr %v", err, tt.wantErr)
			}

			if len(fq.queued) != 1 {
				t.Errorf("Enqueue() queued = %v, want %v", fq.queued, 1)
			}

			if reflect.DeepEqual(tt.args.reply, &EnqueuedCommand{Command: *tt.args.cmd, EnqueuedAt: fq.now}) != true {
				t.Errorf("Enqueue() reply.Command = %v, want %v", tt.args.reply.Command, tt.args.cmd)
			}
		})
	}
}
