package core

import (
	"context"
	"errors"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/sagernet/sing-box/adapter"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/tionmon/sbroute/internal/model"
)

// Native protocol parsers receive an admitted socket; no application-level relay is added.
// Admission happens before parsing so slow/unauthenticated clients cannot create unlimited goroutines.
type handshakeKey struct{}
type admissionRouter struct{ adapter.Router }

func clearHandshake(ctx context.Context) {
	if conn, ok := ctx.Value(handshakeKey{}).(net.Conn); ok {
		_ = conn.SetReadDeadline(time.Time{})
	}
}
func (r *admissionRouter) RouteConnectionEx(ctx context.Context, conn net.Conn, m adapter.InboundContext, done N.CloseHandlerFunc) {
	clearHandshake(ctx)
	r.Router.RouteConnectionEx(ctx, conn, m, done)
}
func (r *admissionRouter) RoutePacketConnectionEx(ctx context.Context, conn N.PacketConn, m adapter.InboundContext, done N.CloseHandlerFunc) {
	clearHandshake(ctx)
	r.Router.RoutePacketConnectionEx(ctx, conn, m, done)
}

type admittedConn struct {
	net.Conn
	once sync.Once
	done func()
	err  error
}

func (c *admittedConn) Close() error {
	c.once.Do(func() { c.err = c.Conn.Close(); c.done() })
	return c.err
}
func (c *admittedConn) ReaderReplaceable() bool { return true }
func (c *admittedConn) WriterReplaceable() bool { return true }
func (c *admittedConn) Upstream() any           { return c.Conn }
func (e *Engine) startGuardedProxy(entry model.Entrance, in adapter.Inbound, scope *adapter.Scope) error {
	handler, ok := in.(adapter.TCPInjectableInbound)
	if !ok {
		return errors.New("core mixed inbound no longer supports native injection")
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(entry.Listen, strconv.Itoa(int(entry.Port))))
	if err != nil {
		return err
	}
	scope.Add(ln.Close)
	go func() {
		var acceptDelay time.Duration
		for {
			raw, err := ln.Accept()
			if err != nil {
				if temporary, ok := err.(net.Error); ok && temporary.Temporary() {
					if acceptDelay == 0 {
						acceptDelay = 5 * time.Millisecond
					} else {
						acceptDelay = min(acceptDelay*2, time.Second)
					}
					select {
					case <-scope.Context().Done():
						return
					case <-time.After(acceptDelay):
						continue
					}
				}
				return
			}
			acceptDelay = 0
			settings := e.snap.Load().config.Resources
			if e.closing.Load() || e.pressure.Load() || !reserve(&e.inboundTCP, int64(settings.MaxSessions+settings.MaxDials)) {
				raw.Close()
				e.rejected.Add(1)
				continue
			}
			current := e.snap.Load().routes[entry.ID]
			if current == nil {
				e.inboundTCP.Add(-1)
				raw.Close()
				continue
			}
			allowed := len(current.prefixes) == 0
			ip := M.SocksaddrFromNet(raw.RemoteAddr()).Addr.Unmap()
			for _, p := range current.prefixes {
				if p.Contains(ip) {
					allowed = true
					break
				}
			}
			if !allowed {
				e.inboundTCP.Add(-1)
				raw.Close()
				continue
			}
			c := &admittedConn{Conn: raw}
			c.done = func() { e.inboundConnections.Delete(c); e.inboundTCP.Add(-1) }
			e.inboundConnections.Store(c, true)
			_ = c.SetReadDeadline(time.Now().Add(10 * time.Second))
			ctx := context.WithValue(e.coreCtx, handshakeKey{}, net.Conn(c))
			meta := adapter.InboundContext{Source: M.SocksaddrFromNet(raw.RemoteAddr()).Unwrap(), OriginDestination: M.SocksaddrFromNet(raw.LocalAddr()).Unwrap()}
			go handler.NewConnection(ctx, c, meta, nil)
		}
	}()
	return nil
}
