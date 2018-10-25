package libp2p

import (
	"bytes"
	"io"
	"time"

	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"
	net "github.com/libp2p/go-libp2p-net"
)

func NewStream(s net.Stream, r, w time.Duration) Stream {
	return Stream{
		s:            s,
		rlp:          rlp.NewStream(s, 0),
		readTimeout:  r,
		writeTimeout: w,
	}
}

type Stream struct {
	readTimeout, writeTimeout time.Duration

	rlp *rlp.Stream
	s   net.Stream
}

func (s Stream) ReadMsg() (msg p2p.Msg, err error) {
	if s.readTimeout != 0 {
		if err = s.s.SetReadDeadline(time.Now().Add(s.readTimeout)); err != nil {
			return
		}
	}
	code, err := s.rlp.Uint()
	if err != nil {
		return
	}
	payload, err := s.rlp.Raw()
	if err != nil {
		return
	}
	return p2p.Msg{
		Code:       code,
		Size:       uint32(len(payload)),
		Payload:    bytes.NewReader(payload),
		ReceivedAt: time.Now(),
	}, nil
}

func (s Stream) WriteMsg(msg p2p.Msg) (err error) {
	if s.writeTimeout != 0 {
		if err = s.s.SetWriteDeadline(time.Now().Add(s.writeTimeout)); err != nil {
			return
		}
	}
	if err = rlp.Encode(s.s, msg.Code); err != nil {
		return
	}
	_, err = io.Copy(s.s, msg.Payload)
	return
}
