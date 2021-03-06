package core

import (
	"encoding/binary"
	"utils/pio"
	"utils/pool"
	"net"
	"time"
)

const (
	_ = iota
	idSetChunkSize /*Set Chunk Size(Message Type ID=1):*/
	idAbortMessage /*Abort Message(Message Type ID=2)*/
	idAck  /*Acknowledgement(Message Type ID=3)*/
	idUserControlMessages
	idWindowAckSize
	idSetPeerBandwidth
)

type Conn struct { /*a wrapper for network connection */
	net.Conn
	chunkSize           uint32
	remoteChunkSize     uint32
	windowAckSize       uint32
	remoteWindowAckSize uint32
	received            uint32
	ackReceived         uint32
	rw                  *ReadWriter
	pool                *pool.Pool
	chunks              map[uint32]ChunkStream /*key is csid */
}

func NewConn(c net.Conn, bufferSize int) *Conn {
	return &Conn{
		Conn:                c,
		chunkSize:           128,
		remoteChunkSize:     128,
		windowAckSize:       2500000,
		remoteWindowAckSize: 2500000,
		pool:                pool.NewPool(),
		rw:                  NewReadWriter(c, bufferSize),
		chunks:              make(map[uint32]ChunkStream),
	}
}
/*chunk stream basic header : fmt csid */
func (conn *Conn) Read(c *ChunkStream) error {
	for {
		/*the first byte in BE is the fix size basic header : basic header是此包的唯一不变的部分,并且由一个独立的byte构成,这其中包括了2个作重要的标志位,chunk type以及stream id.chunk type决定了消息头的编码格式,该字段的长度完全依赖于stream id,stream id是一个可变长的字段.*/
		h, _ := conn.rw.ReadUintBE(1) /*chunk 's basic head is 1 byte ,then csid is 6 bits*/
		// if err != nil {
		// 	log.Println("read from conn error: ", err)
		// 	return err
		// }
		format := h >> 6 /*2 Bits chunk type  */
		csid := h & 0x3f /*chunk stream id : 6 bit*/
		cs, ok := conn.chunks[csid]
		if !ok {
			cs = ChunkStream{}
			conn.chunks[csid] = cs  /* create and insert a chunk stream*/
		}
		/*read basic header */
		cs.tmpFromat = format  /*fmt : chunk type*/
		cs.CSID = csid  /*stream id*/
		/*read message header and data*/
		err := cs.readChunk(conn.rw, conn.remoteChunkSize, conn.pool)
		if err != nil {
			return err
		}
		conn.chunks[csid] = cs
		if cs.full() {
			*c = cs /*we got this chunk stream */
			break /*break for loop */
		}
	}
	/*Protocol Control Message*/
	conn.handleControlMsg(c)

	conn.ack(c.Length)

	return nil
}

func (conn *Conn) Write(c *ChunkStream) error {
	if c.TypeID == idSetChunkSize {
		conn.chunkSize = binary.BigEndian.Uint32(c.Data)
	}
	return c.writeChunk(conn.rw, int(conn.chunkSize))
}

func (conn *Conn) Flush() error {
	return conn.rw.Flush()
}

func (conn *Conn) Close() error {
	return conn.Conn.Close()
}

func (conn *Conn) RemoteAddr() net.Addr {
	return conn.Conn.RemoteAddr()
}

func (conn *Conn) LocalAddr() net.Addr {
	return conn.Conn.LocalAddr()
}

func (conn *Conn) SetDeadline(t time.Time) error {
	return conn.Conn.SetDeadline(t)
}

func (conn *Conn) NewAck(size uint32) ChunkStream {
	return initControlMsg(idAck, 4, size)
}

func (conn *Conn) NewSetChunkSize(size uint32) ChunkStream {
	return initControlMsg(idSetChunkSize, 4, size)
}

func (conn *Conn) NewWindowAckSize(size uint32) ChunkStream {
	return initControlMsg(idWindowAckSize, 4, size)
}

func (conn *Conn) NewSetPeerBandwidth(size uint32) ChunkStream {
	ret := initControlMsg(idSetPeerBandwidth, 5, size)
	ret.Data[4] = 2
	return ret
}

func (conn *Conn) handleControlMsg(c *ChunkStream) {
	if c.TypeID == idSetChunkSize {
		conn.remoteChunkSize = binary.BigEndian.Uint32(c.Data)
	} else if c.TypeID == idWindowAckSize {
		/*当收到对端的消息大小等于窗口大小（Window Size）时接受端要回馈一个ACK给发送端告知对方可以继续发送数据。窗口大小就是指收到接受端返回
的ACK前最多可以发送的字节数量，返回的ACK中会带有从发送上一个ACK后接收到的字节数*/
		conn.remoteWindowAckSize = binary.BigEndian.Uint32(c.Data)
	}
}

func (conn *Conn) ack(size uint32) {
	conn.received += uint32(size)
	conn.ackReceived += uint32(size)
	if conn.received >= 0xf0000000 {
		conn.received = 0
	}
	if conn.ackReceived >= conn.remoteWindowAckSize {
		cs := conn.NewAck(conn.ackReceived)
		cs.writeChunk(conn.rw, int(conn.chunkSize))
		conn.ackReceived = 0
	}
}

func initControlMsg(id, size, value uint32) ChunkStream {
	ret := ChunkStream{
		Format:   0,
		CSID:     2,
		TypeID:   id,
		StreamID: 0,
		Length:   size,
		Data:     make([]byte, size),
	}
	pio.PutU32BE(ret.Data[:size], value)
	return ret
}

const (
	streamBegin      uint32 = 0
	streamEOF        uint32 = 1
	streamDry        uint32 = 2
	setBufferLen     uint32 = 3
	streamIsRecorded uint32 = 4
	pingRequest      uint32 = 6
	pingResponse     uint32 = 7
)

/*
   +------------------------------+-------------------------
   |     Event Type ( 2- bytes )  | Event Data
   +------------------------------+-------------------------
   Pay load for the ‘User Control Message’.
*/
func (conn *Conn) userControlMsg(eventType, buflen uint32) ChunkStream {
	var ret ChunkStream
	buflen += 2
	ret = ChunkStream{
		Format:   0,
		CSID:     2,
		TypeID:   4,
		StreamID: 1,
		Length:   buflen,
		Data:     make([]byte, buflen),
	}
	ret.Data[0] = byte(eventType >> 8 & 0xff)
	ret.Data[1] = byte(eventType & 0xff)
	return ret
}

func (conn *Conn) SetBegin() {
	ret := conn.userControlMsg(streamBegin, 4)
	for i := 0; i < 4; i++ {
		ret.Data[2+i] = byte(1 >> uint32((3-i)*8) & 0xff)
	}
	conn.Write(&ret)
}

func (conn *Conn) SetRecorded() {
	ret := conn.userControlMsg(streamIsRecorded, 4)
	for i := 0; i < 4; i++ {
		ret.Data[2+i] = byte(1 >> uint32((3-i)*8) & 0xff)
	}
	conn.Write(&ret)
}
