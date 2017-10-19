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
	Timestamp uint32 /*4 bytes*/
	Length    uint32 /*payload size in 3 bytes*/
	TypeID    uint32 /*1 bytes*/
	StreamID  uint32 /*message stream id ，4 bytes in LE*/
	timeDelta uint32
	exted     bool
	index     uint32
	remain    uint32
	got       bool
	tmpFormat uint32 /*chunck stream type*/
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
需要注意的是，Basic Header是采用小端存储的方式，越往后的字节数量级越高，因此通过这3个字节每一位的值来计算CSID时，应该是:<第三个字节的值>x256+<第二个字节的值>+64
針對 FMT=1 或 2 的格式，若兩個 chunk 的 timestamp 差異大於 16777215 (hexadecimal 0xFFFFFF)，表示此時封包會存在 Extended Timestamp field
*/
func (chunkStream *ChunkStream) readChunk(r *ReadWriter, chunkSize uint32, pool *pool.Pool) error {
	if chunkStream.remain != 0 && chunkStream.tmpFormat != 3 {
		return fmt.Errorf("invalid remin = %d", chunkStream.remain)
	}
	/*-------------------------------------------------------------*/

	/*chunk stream id decides how many bytes the BasicHeader has: http://blog.csdn.net/stn_lcd/article/details/72901722
3及以上的则Basic header为一个字节
0为两个字节，chunk stream id = 64 + 第二个字节值 （64-319）
1为三个字节，chunk stream id = 第三字节*256 + 第二字节 + 64（64–65599）
2为一个字节，Value 2 indicates its low-level protocol message
	*/
	switch chunkStream.CSID { /*0，1，2由协议保留表示特殊信息。0代表Basic Header总共要占用2个字节，CSID在［64，319］之间，1代表占用3个字节，CSID在［64，65599］之间，2代表该
chunk是控制信息和一些命令信息 注意: 當 Stream ID = 2 時有特殊用途。Chunk Stream ID with value 2 is reserved for low-level protocol control messages and commands.*/
	case 0:
		id, _ := r.ReadUintLE(1) /*read another 1 byte ,in little endian*/
		chunkStream.CSID = id + 64
	case 1:
		id, _ := r.ReadUintLE(2) /*read another two bytes ,in little endian */
		chunkStream.CSID = id + 64
	}

	/*-------------------------------------------------------------*/

	/*fmt decides the length of header(chunk message header ,no include basic header size )
	 两位的fmt取值为 0~3，分别代表的意义如下：
      case 0：chunk Msg Header长度为11；
      case 1：chunk Msg Header长度为7；
      case 2：chunk Msg Header长度为3；
      case 3：chunk Msg Header长度为0；
	*/
	/*Message Header的格式和长度取决于Basic Header的fmt (chunk type)，共有4种不同的格式(0 1 2 3 )，由上面所提到的Basic Header中的fmt字段控制*/
	switch chunkStream.tmpFormat {
	case 0: /*FMT = 0，message header = timestamp(3) |mesage_length(3) |mesage_type_id(1) | msg_stream_id(4) */
		chunkStream.Format = chunkStream.tmpFormat
		chunkStream.Timestamp, _ = r.ReadUintBE(3)
		chunkStream.Length, _ = r.ReadUintBE(3)
		chunkStream.TypeID, _ = r.ReadUintBE(1)
		chunkStream.StreamID, _ = r.ReadUintLE(4)
		if chunkStream.Timestamp == 0xffffff { /*timestamp扩展时间戳是当chunk message header的时间戳大于等于0xffffff的时候chunk message header后面的四个字节就代表扩展时间.*/
			chunkStream.Timestamp, _ = r.ReadUintBE(4)
			chunkStream.exted = true
		} else {
			chunkStream.exted = false
		}
		chunkStream.new(pool)
	case 1: /*FMT = 1，message header = timestamp_delta(3) |mesage_length(3) |mesage_type_id(1) 接著因為後續的封包屬於同一條 stream, 可以省略 stream_id(4)，只送出 7 bytes的 message header*/
		chunkStream.Format = chunkStream.tmpFormat
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
	case 2: /*FMT = 2，message header = timestamp_delta(3) 如果是固定長度訊息(constant-sized messages)，例如:audio stream，那麼可以再省略 mesage_length(3) |mesage_type_id(1)，只送出 3 bytes 的 message header。 */
		chunkStream.Format = chunkStream.tmpFormat
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
	case 3: /*FMT = 3，no message header 若單一訊息被拆成多個 chunks, 後續 chunks 可沿用第一個 chunk的所有資訊。此時應該使用此格式。When a single message is split into chunks, all chunks of a message except the first one SHOULD use this type.*/
		/*0字节！！！好吧，它表示这个chunk的Message Header和上一个是完全相同的，自然就不用再传输一遍了。当它跟在Type＝0的chunk后面时，表示和前一个chunk的时间戳都是相同的。
				什么时候连时间戳都相同呢？就是一个Message拆分成了多个chunk，这个chunk和上一个chunk同属于一个Message。 */
		if chunkStream.remain == 0 {
			switch chunkStream.Format {
			case 0:
				if chunkStream.exted {
					timestamp, _ := r.ReadUintBE(4)
					chunkStream.Timestamp = timestamp
				}
				/*而当它跟在Type＝1或者Type＝2的chunk后面时，表示和前一个chunk的时间戳的差是相同的。比如第一个chunk的Type＝0，timestamp＝100，第二个chunk的Type＝2，timestamp
delta＝20，表示时间戳为100+20=120，第三个chunk的Type＝3，表示timestamp delta＝20，时间戳为120+20=140*/
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
		} else {/*has more data ??*/
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
	/*-------------------------------------------------------------*/
	size := int(chunkStream.remain)
	if size > int(chunkSize) {
		size = int(chunkSize)
	}

	buf := chunkStream.Data[chunkStream.index: chunkStream.index+uint32(size)]
	/*read from []byte to chunk data buffer */
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
