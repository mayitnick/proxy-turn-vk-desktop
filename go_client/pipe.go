package clientengine

import (
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

type pipePacket struct {
	data []byte
	addr net.Addr
}

type asyncPacketConn struct {
	readCh  chan pipePacket
	writeCh chan pipePacket
	local   net.Addr
	remote  net.Addr
	closed  chan struct{}
	once    sync.Once
}

type pipeAddr struct {
	name string
}

func (a pipeAddr) Network() string { return "pipe" }
func (a pipeAddr) String() string  { return a.name }

func AsyncPacketPipe() (net.PacketConn, net.PacketConn) {
	chA := make(chan pipePacket, 512)
	chB := make(chan pipePacket, 512)

	addrA := pipeAddr{"pipeA"}
	addrB := pipeAddr{"pipeB"}

	connA := &asyncPacketConn{
		readCh:  chA,
		writeCh: chB,
		local:   addrA,
		remote:  addrB,
		closed:  make(chan struct{}),
	}

	connB := &asyncPacketConn{
		readCh:  chB,
		writeCh: chA,
		local:   addrB,
		remote:  addrA,
		closed:  make(chan struct{}),
	}

	return connA, connB
}

func (c *asyncPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	select {
	case pkt, ok := <-c.readCh:
		if !ok {
			return 0, nil, io.EOF
		}
		n = copy(p, pkt.data)
		return n, pkt.addr, nil
	case <-c.closed:
		return 0, nil, errors.New("use of closed network connection")
	}
}

func (c *asyncPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	select {
	case <-c.closed:
		return 0, errors.New("use of closed network connection")
	default:
	}

	buf := make([]byte, len(p))
	copy(buf, p)

	select {
	case c.writeCh <- pipePacket{data: buf, addr: c.local}:
		return len(p), nil
	case <-c.closed:
		return 0, errors.New("use of closed network connection")
	}
}

func (c *asyncPacketConn) Close() error {
	c.once.Do(func() {
		close(c.closed)
	})
	return nil
}

func (c *asyncPacketConn) LocalAddr() net.Addr                { return c.local }
func (c *asyncPacketConn) SetDeadline(t time.Time) error      { return nil }
func (c *asyncPacketConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *asyncPacketConn) SetWriteDeadline(t time.Time) error { return nil }
