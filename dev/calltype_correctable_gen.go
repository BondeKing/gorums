// Code generated by 'gorums' plugin for protoc-gen-go. DO NOT EDIT.
// Source file to edit is: calltype_correctable_tmpl

package dev

import (
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"golang.org/x/net/context"
)

/* Exported types and methods for correctable method ReadCorrectable */

// ReadCorrectableReply is a reference to a correctable ReadCorrectable quorum call.
type ReadCorrectableReply struct {
	mu sync.Mutex
	// the actual reply
	*State
	NodeIDs  []uint32
	level    int
	err      error
	done     bool
	watchers []*struct {
		level int
		ch    chan struct{}
	}
	donech chan struct{}
}

// ReadCorrectable asynchronously invokes a
// correctable ReadCorrectable quorum call on configuration c and returns a
// ReadCorrectableReply which can be used to inspect any replies or errors
// when available.
func (c *Configuration) ReadCorrectable(ctx context.Context, args *ReadRequest) *ReadCorrectableReply {
	corr := &ReadCorrectableReply{
		level:   LevelNotSet,
		NodeIDs: make([]uint32, 0, c.n),
		donech:  make(chan struct{}),
	}
	go func() {
		c.readCorrectable(ctx, args, corr)
	}()
	return corr
}

// Get returns the reply, level and any error associated with the
// ReadCorrectable. The method does not block until a (possibly
// itermidiate) reply or error is available. Level is set to LevelNotSet if no
// reply has yet been received. The Done or Watch methods should be used to
// ensure that a reply is available.
func (c *ReadCorrectableReply) Get() (*State, int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.State, c.level, c.err
}

// Done returns a channel that's closed when the correctable ReadCorrectable
// quorum call is done. A call is considered done when the quorum function has
// signaled that a quorum of replies was received or that the call returned an
// error.
func (c *ReadCorrectableReply) Done() <-chan struct{} {
	return c.donech
}

// Watch returns a channel that's closed when a reply or error at or above the
// specified level is available. If the call is done, the channel is closed
// disregardless of the specified level.
func (c *ReadCorrectableReply) Watch(level int) <-chan struct{} {
	ch := make(chan struct{})
	c.mu.Lock()
	if level < c.level {
		close(ch)
		c.mu.Unlock()
		return ch
	}
	c.watchers = append(c.watchers, &struct {
		level int
		ch    chan struct{}
	}{level, ch})
	c.mu.Unlock()
	return ch
}

func (c *ReadCorrectableReply) set(reply *State, level int, err error, done bool) {
	c.mu.Lock()
	if c.done {
		c.mu.Unlock()
		panic("set(...) called on a done correctable")
	}
	c.State, c.level, c.err, c.done = reply, level, err, done
	if done {
		close(c.donech)
		for _, watcher := range c.watchers {
			if watcher != nil {
				close(watcher.ch)
			}
		}
		c.mu.Unlock()
		return
	}
	for i := range c.watchers {
		if c.watchers[i] != nil && c.watchers[i].level <= level {
			close(c.watchers[i].ch)
			c.watchers[i] = nil
		}
	}
	c.mu.Unlock()
}

/* Unexported types and methods for correctable method ReadCorrectable */

type readCorrectableReply struct {
	nid   uint32
	reply *State
	err   error
}

func (c *Configuration) readCorrectable(ctx context.Context, a *ReadRequest, resp *ReadCorrectableReply) {
	replyChan := make(chan readCorrectableReply, c.n)
	var wg sync.WaitGroup
	wg.Add(c.n)
	for _, n := range c.nodes {
		go callGRPCReadCorrectable(ctx, &wg, n, a, replyChan)
	}
	wg.Wait()

	var (
		replyValues = make([]*State, 0, c.n)
		clevel      = LevelNotSet
		reply       *State
		rlevel      int
		errCount    int
		quorum      bool
	)

	for {
		select {
		case r := <-replyChan:
			resp.NodeIDs = append(resp.NodeIDs, r.nid)
			if r.err != nil {
				errCount++
				break
			}
			replyValues = append(replyValues, r.reply)
			reply, rlevel, quorum = c.qspec.ReadCorrectableQF(replyValues)
			if quorum {
				resp.set(reply, rlevel, nil, true)
				return
			}
			if rlevel > clevel {
				clevel = rlevel
				resp.set(reply, rlevel, nil, false)
			}
		case <-ctx.Done():
			resp.set(reply, clevel, QuorumCallError{ctx.Err().Error(), errCount, len(replyValues)}, true)
			return
		}

		if errCount+len(replyValues) == c.n {
			resp.set(reply, clevel, QuorumCallError{"incomplete call", errCount, len(replyValues)}, true)
			return
		}
	}
}

func callGRPCReadCorrectable(ctx context.Context, wg *sync.WaitGroup, node *Node, arg *ReadRequest, replyChan chan<- readCorrectableReply) {
	wg.Done()
	reply := new(State)
	start := time.Now()
	err := grpc.Invoke(
		ctx,
		"/dev.Storage/ReadCorrectable",
		arg,
		reply,
		node.conn,
	)
	switch grpc.Code(err) { // nil -> codes.OK
	case codes.OK, codes.Canceled:
		node.setLatency(time.Since(start))
	default:
		node.setLastErr(err)
	}
	replyChan <- readCorrectableReply{node.id, reply, err}
}
