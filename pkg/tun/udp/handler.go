package udp

import (
	"context"
	"sync"
	"time"

	"github.com/datawire/dlib/dlog"
	"github.com/telepresenceio/telepresence/v2/pkg/connpool"
	"github.com/telepresenceio/telepresence/v2/pkg/tun/ip"
)

type DatagramHandler interface {
	connpool.Handler
	NewDatagram(ctx context.Context, dg Datagram)
}

type handler struct {
	connpool.Tunnel
	id        connpool.ConnID
	remove    func()
	toTun     chan<- ip.Packet
	fromTun   chan Datagram
	idleTimer *time.Timer
	idleLock  sync.Mutex
}

const ioChannelSize = 0x40
const idleDuration = time.Second

func (h *handler) NewDatagram(ctx context.Context, dg Datagram) {
	select {
	case <-ctx.Done():
	case h.fromTun <- dg:
	}
}

func (h *handler) Close(_ context.Context) {
	h.remove()
}

func NewHandler(tunnel connpool.Tunnel, toTun chan<- ip.Packet, id connpool.ConnID, remove func()) DatagramHandler {
	return &handler{
		Tunnel:  tunnel,
		id:      id,
		toTun:   toTun,
		remove:  remove,
		fromTun: make(chan Datagram, ioChannelSize),
	}
}

func (h *handler) HandleMessage(ctx context.Context, mdg connpool.Message) {
	payload := mdg.Payload()
	pkt := NewDatagram(HeaderLen+len(payload), h.id.Destination(), h.id.Source())
	ipHdr := pkt.IPHeader()
	ipHdr.SetChecksum()

	udpHdr := Header(ipHdr.Payload())
	udpHdr.SetSourcePort(h.id.DestinationPort())
	udpHdr.SetDestinationPort(h.id.SourcePort())
	udpHdr.SetPayloadLen(uint16(len(payload)))
	copy(udpHdr.Payload(), payload)
	udpHdr.SetChecksum(ipHdr)

	select {
	case <-ctx.Done():
		return
	case h.toTun <- pkt:
	}
}

func (h *handler) Start(ctx context.Context) {
	h.idleTimer = time.NewTimer(idleDuration)
	go h.writeLoop(ctx)
}

func (h *handler) writeLoop(ctx context.Context) {
	defer h.Close(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-h.idleTimer.C:
			return
		case dg := <-h.fromTun:
			if !h.resetIdle() {
				return
			}
			dlog.Debugf(ctx, "<- TUN %s", dg)
			dlog.Debugf(ctx, "-> MGR %s", dg)
			udpHdr := dg.Header()
			err := h.Send(ctx, connpool.NewMessage(h.id, udpHdr.Payload()))
			dg.SoftRelease()
			if err != nil {
				if ctx.Err() == nil {
					dlog.Errorf(ctx, "failed to send ConnMessage: %v", err)
				}
				return
			}
		}
	}
}

func (h *handler) resetIdle() bool {
	h.idleLock.Lock()
	stopped := h.idleTimer.Stop()
	if stopped {
		h.idleTimer.Reset(idleDuration)
	}
	h.idleLock.Unlock()
	return stopped
}
