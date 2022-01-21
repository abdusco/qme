# qme (queue me)

A simple queueing system for long-running commands. It allows you to queue up shell commands from anywhere, and run them
in order.

This is useful for enqueueing long-running commands sharing a limited resource, like a video encoding (which maxes out
CPU), rsync'ing files (which take up all upload bandwidth), or running a build script (which takes up all CPU).

If the program you're running has no built-in queueing functionality, and you have no pre-determined list of jobs,
such that running a shell script isn't an option, this is a simple way to get it done.

## Usage

In any terminal simply prefix your command with `qme`

```shell
sleep 5  # executes the command directly
./qme sleep 5  # queues and executes the command
```

This will enqueue the command and start executing it right away, piping its `stdout` and `stderr` to the terminal, but
it will also keep an RPC server running in the background.

```shell
$ ./qme sleep 5
2022/01/21 10:54:12 enqueueing 'sleep'
2022/01/21 10:54:12 assuming server role
2022/01/21 10:54:12 listening on /tmp/qme.sock
2022/01/21 10:54:12 started executing 'sleep' with pid 62775
2022/01/21 10:28:30 finished: exit status 0
2022/01/21 10:28:30 idling...
2022/01/21 10:28:50 idle timeout reached, shutting down
```

So when you enqueue another task before the server process shuts down (it timeouts in 20s), it will connect & enqueue
the command on the server process, and it will be executed there.

```shell
# this will be executed on the server process
$ ./qme sleep 1
2022/01/21 10:54:13 connected. assuming client role
2022/01/21 10:54:13 enqueued 'sleep'

$ ./qme sleep 2
2022/01/21 10:54:14 connected. assuming client role
2022/01/21 10:54:14 enqueued 'sleep'
```

```shell
# server process accepts the command, and starts executing it  
...
2022/01/21 10:30:36 idling...
2022/01/21 10:54:13 enqueueing 'sleep'  # <-- command accepted into the queue
2022/01/21 10:54:14 enqueueing 'sleep'  # <-- 
2022/01/21 10:54:22 finished: exit status 0
2022/01/21 10:54:22 idling...
2022/01/21 10:54:22 started executing 'sleep' with pid 62804 # <-- command is now executing
2022/01/21 10:54:23 finished: exit status 0
2022/01/21 10:54:23 idling...
2022/01/21 10:54:23 started executing 'sleep' with pid 62805
2022/01/21 10:54:25 finished: exit status 0
2022/01/21 10:54:25 idling...
2022/01/21 10:54:45 idle timeout reached, shutting down # <-- server process shuts down if there's nothing to do
```

If the server already shut down, it will assume the server role and start executing & listening again. So no matter when
& where you run `qme`, it will run the command now or after other queued commands finishes executing, but not at the
same time.

## TODO

- [x] Add tests
- [ ] Respect os signals
- [ ] Make idle timeout configurable
- [ ] Support separate queues (e.g. one queue for CPU-heavy, another one for network-heavy, etc.)
- [ ] Support command weights, so that important commands are executed first