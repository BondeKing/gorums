package dev

import "fmt"

// ReadReply encapsulates the reply from a Read RPC invocation.
// It contains the id of each node in the quorum that replied and a single
// reply.
type ReadReply struct {
	NodeIDs []uint32
	Reply   *State
}

func (r ReadReply) String() string {
	return fmt.Sprintf("node ids: %v | answer: %v", r.NodeIDs, r.Reply)
}

// Read invokes a Read RPC on configuration c
// and returns the result as a ReadReply.
func (c *Configuration) Read(args *ReadRequest) (*ReadReply, error) {
	return c.mgr.read(c, args)
}

// ReadFuture is a reference to an asynchronous Read RPC invocation.
type ReadFuture struct {
	reply *ReadReply
	err   error
	c     chan struct{}
}

// ReadFuture asynchronously invokes a Read RPC on configuration c and
// returns a ReadFuture which can be used to inspect the RPC reply and error
// when available.
func (c *Configuration) ReadFuture(args *ReadRequest) *ReadFuture {
	f := new(ReadFuture)
	f.c = make(chan struct{}, 1)
	go func() {
		defer close(f.c)
		f.reply, f.err = c.mgr.read(c, args)
	}()
	return f
}

// Get returns the reply and any error associated with the ReadFuture.
// The method blocks until a reply or error is available.
func (f *ReadFuture) Get() (*ReadReply, error) {
	<-f.c
	return f.reply, f.err
}

// Done reports if a reply or error is available for the ReadFuture.
func (f *ReadFuture) Done() bool {
	select {
	case <-f.c:
		return true
	default:
		return false
	}
}

// WriteReply encapsulates the reply from a Write RPC invocation.
// It contains the id of each node in the quorum that replied and a single
// reply.
type WriteReply struct {
	NodeIDs []uint32
	Reply   *WriteResponse
}

func (r WriteReply) String() string {
	return fmt.Sprintf("node ids: %v | answer: %v", r.NodeIDs, r.Reply)
}

// Write invokes a Write RPC on configuration c
// and returns the result as a WriteReply.
func (c *Configuration) Write(args *State) (*WriteReply, error) {
	return c.mgr.write(c, args)
}

// WriteFuture is a reference to an asynchronous Write RPC invocation.
type WriteFuture struct {
	reply *WriteReply
	err   error
	c     chan struct{}
}

// WriteFuture asynchronously invokes a Write RPC on configuration c and
// returns a WriteFuture which can be used to inspect the RPC reply and error
// when available.
func (c *Configuration) WriteFuture(args *State) *WriteFuture {
	f := new(WriteFuture)
	f.c = make(chan struct{}, 1)
	go func() {
		defer close(f.c)
		f.reply, f.err = c.mgr.write(c, args)
	}()
	return f
}

// Get returns the reply and any error associated with the WriteFuture.
// The method blocks until a reply or error is available.
func (f *WriteFuture) Get() (*WriteReply, error) {
	<-f.c
	return f.reply, f.err
}

// Done reports if a reply or error is available for the WriteFuture.
func (f *WriteFuture) Done() bool {
	select {
	case <-f.c:
		return true
	default:
		return false
	}
}

// WriteAsync invokes an asynchronous WriteAsync RPC on configuration c.
// The call has no return value and is invoked on every node in the
// configuration.
func (c *Configuration) WriteAsync(args *State) error {
	return c.mgr.writeAsync(c, args)
}
