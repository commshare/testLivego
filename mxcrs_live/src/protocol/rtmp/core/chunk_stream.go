package core

import (
	"encoding/binary"
	"fmt"

	"av"
	"utils/pool"
)

type ChunkStream struct {
	Format    uint32
	CSID      uint32
	Timestamp uint32
	Length    uint32
	TypeID    uint32
	StreamID  uint32
	timeDelta uint32
	exted     bool
	index     uint32
	remain    uint32
	got       bool
	tmpFromat uint32
	Data      []byte
}

func (chunkStream *ChunkStream) full() bool {
	return chunkStream.got
}

func (chunkStream *ChunkStream) new(pool *pool.Pool) {
	chunkStream.got = false /*not receive all chunk data*/
	chunkStream.index = 0
	chunkStream.remain = chunkStream.Length /*init value ？*/
	chunkStream.Data = pool.Get(int(chunkStream.Length))
}

func (chunkStream *ChunkStream) writeHeader(w *ReadWriter) error {
	//Chunk Basic Header
	h := chunkStream.Format << 6
	switch {
	case chunkStream.CSID < 64:
		h |= chunkStream.CSID
		w.WriteUintBE(h, 1)
	case chunkStream.CSID-64 < 256:
		h |= 0
		w.WriteUintBE(h, 1)
		w.WriteUintLE(chunkStream.CSID-64, 1)
	case chunkStream.CSID-64 < 65536:
		h |= 1
		w.WriteUintBE(h, 1)
		w.WriteUintLE(chunkStream.CSID-64, 2)
	}
	//Chunk Message Header
	ts := chunkStream.Timestamp
	if chunkStream.Format == 3 {
		goto END
	}
	if chunkStream.Timestamp > 0xffffff {
		ts = 0xffffff
	}
	w.WriteUintBE(ts, 3)
	if chunkStream.Format == 2 {
		goto END
	}
	if chunkStream.Length > 0xffffff {
		return fmt.Errorf("length=%d", chunkStream.Length)
	}
	w.WriteUintBE(chunkStream.Length, 3)
	w.WriteUintBE(chunkStream.TypeID, 1)
	if chunkStream.Format == 1 {
		goto END
	}
	w.WriteUintLE(chunkStream.StreamID, 4)
END:
//Extended Timestamp
	if ts >= 0xffffff {
		w.WriteUintBE(chunkStream.Timestamp, 4)
	}
	return w.WriteError()
}

func (chunkStream *ChunkStream) writeChunk(w *ReadWriter, chunkSize int) error {
	if chunkStream.TypeID == av.TAG_AUDIO {
		chunkStream.CSID = 4
	} else if chunkStream.TypeID == av.TAG_VIDEO ||
		chunkStream.TypeID == av.TAG_SCRIPTDATAAMF0 ||
		chunkStream.TypeID == av.TAG_SCRIPTDATAAMF3 {
		chunkStream.CSID = 6
	}

	totalLen := uint32(0)
	numChunks := (chunkStream.Length / uint32(chunkSize))
	for i := uint32(0); i <= numChunks; i++ {
		if totalLen == chunkStream.Length {
			break
		}
		if i == 0 {
			chunkStream.Format = uint32(0)
		} else {
			chunkStream.Format = uint32(3)
		}
		if err := chunkStream.writeHeader(w); err != nil {
			return err
		}
		inc := uint32(chunkSize)
		start := uint32(i) * uint32(chunkSize)
		if uint32(len(chunkStream.Data))-start <= inc {
			inc = uint32(len(chunkStream.Data)) - start
		}
		totalLen += inc
		end := start + inc
		buf := chunkStream.Data[start:end]
		if _, err := w.Write(buf); err != nil {
			return err
		}
	}

	return nil

}
/*
需要注意的是，Basic Header是采用小端存储的方
式，越往后的字节数量级越高，因此通过这3个字节每一位的值来计算CSID
时，应该是:<第三个字节的值>x256+<第二个字节的值>+64
*/
func (chunkStream *ChunkStream) readChunk(r *ReadWriter, chunkSize uint32, pool *pool.Pool) error {
	if chunkStream.remain != 0 && chunkStream.tmpFromat != 3 {
		return fmt.Errorf("inlaid remin = %d", chunkStream.remain)
	}
	switch chunkStream.CSID { /*0，1，2由协议保留表示特殊信息。0代表Basic Header总共要占用2个字节，CSID在［64，319］之间，1代表占用3个字节，CSID在［64，65599］之间，2代表该
chunk是控制信息和一些命令信息，后面会有详细的介绍*/
	case 0:
		id, _ := r.ReadUintLE(1) /*read another 1 byte ,in little endian*/
		chunkStream.CSID = id + 64
	case 1:
		id, _ := r.ReadUintLE(2) /*read another two bytes ,in little endian */
		chunkStream.CSID = id + 64
	}
	/*Message Header的格式和长度取决于Basic Header的chunk type，共有4种不同的格式，由上面所提到的Basic Header中的fmt字段控制*/
	switch chunkStream.tmpFromat {
	case 0:
		chunkStream.Format = chunkStream.tmpFromat
		chunkStream.Timestamp, _ = r.ReadUintBE(3)
		chunkStream.Length, _ = r.ReadUintBE(3)
		chunkStream.TypeID, _ = r.ReadUintBE(1)
		chunkStream.StreamID, _ = r.ReadUintLE(4)
		if chunkStream.Timestamp == 0xffffff {
			chunkStream.Timestamp, _ = r.ReadUintBE(4)
			chunkStream.exted = true
		} else {
			chunkStream.exted = false
		}
		chunkStream.new(pool)
	case 1:
		chunkStream.Format = chunkStream.tmpFromat
		timeStamp, _ := r.ReadUintBE(3)
		chunkStream.Length, _ = r.ReadUintBE(3)
		chunkStream.TypeID, _ = r.ReadUintBE(1)
		if timeStamp == 0xffffff {
			timeStamp, _ = r.ReadUintBE(4)
			chunkStream.exted = true
		} else {
			chunkStream.exted = false
		}
		chunkStream.timeDelta = timeStamp
		chunkStream.Timestamp += timeStamp
		chunkStream.new(pool)
	case 2:
		chunkStream.Format = chunkStream.tmpFromat
		timeStamp, _ := r.ReadUintBE(3)
		if timeStamp == 0xffffff {
			timeStamp, _ = r.ReadUintBE(4)
			chunkStream.exted = true
		} else {
			chunkStream.exted = false
		}
		chunkStream.timeDelta = timeStamp
		chunkStream.Timestamp += timeStamp
		chunkStream.new(pool)
	case 3:
		if chunkStream.remain == 0 {
			switch chunkStream.Format {
			/*0字节！！！好吧，它表示这个chunk的Message Header和上一个是完
全相同的，自然就不用再传输一遍了。当它跟在Type＝0的chunk后面
时，表示和前一个chunk的时间戳都是相同的。什么时候连时间戳都相同
呢？就是一个Message拆分成了多个chunk，这个chunk和上一个chunk
同属于一个Message。*/
			case 0:
				if chunkStream.exted {
					timestamp, _ := r.ReadUintBE(4)
					chunkStream.Timestamp = timestamp
				}
				/*而当它跟在Type＝1或者Type＝2的chunk后面
时，表示和前一个chunk的时间戳的差是相同的。比如第一个chunk的
Type＝0，timestamp＝100，第二个chunk的Type＝2，timestamp
delta＝20，表示时间戳为100+20=120，第三个chunk的Type＝3，表
示timestamp delta＝20，时间戳为120+20=140*/
			case 1, 2:
				var timedet uint32
				if chunkStream.exted {
					timedet, _ = r.ReadUintBE(4)
				} else {
					timedet = chunkStream.timeDelta
				}
				chunkStream.Timestamp += timedet
			}
			chunkStream.new(pool)
		} else {
			if chunkStream.exted {
				b, err := r.Peek(4)
				if err != nil {
					return err
				}
				tmpts := binary.BigEndian.Uint32(b)
				if tmpts == chunkStream.Timestamp {
					r.Discard(4)
				}
			}
		}
	default:
		return fmt.Errorf("invalid format=%d", chunkStream.Format)
	}
	size := int(chunkStream.remain)
	if size > int(chunkSize) {
		size = int(chunkSize)
	}

	buf := chunkStream.Data[chunkStream.index: chunkStream.index+uint32(size)]
	if _, err := r.Read(buf); err != nil {
		return err
	}
	chunkStream.index += uint32(size)
	chunkStream.remain -= uint32(size)
	if chunkStream.remain == 0 {  /*not data left in the chunk data */
		chunkStream.got = true
	}

	return r.readError
}
