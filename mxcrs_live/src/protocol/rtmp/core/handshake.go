package core

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"

	"time"

	"utils/pio"
)

var (
	timeout = 5 * time.Second
)

var (
	hsClientFullKey = []byte{
		'G', 'e', 'n', 'u', 'i', 'n', 'e', ' ', 'A', 'd', 'o', 'b', 'e', ' ',
		'F', 'l', 'a', 's', 'h', ' ', 'P', 'l', 'a', 'y', 'e', 'r', ' ',
		'0', '0', '1',
		0xF0, 0xEE, 0xC2, 0x4A, 0x80, 0x68, 0xBE, 0xE8, 0x2E, 0x00, 0xD0, 0xD1,
		0x02, 0x9E, 0x7E, 0x57, 0x6E, 0xEC, 0x5D, 0x2D, 0x29, 0x80, 0x6F, 0xAB,
		0x93, 0xB8, 0xE6, 0x36, 0xCF, 0xEB, 0x31, 0xAE,
	}
	hsServerFullKey = []byte{
		'G', 'e', 'n', 'u', 'i', 'n', 'e', ' ', 'A', 'd', 'o', 'b', 'e', ' ',
		'F', 'l', 'a', 's', 'h', ' ', 'M', 'e', 'd', 'i', 'a', ' ',
		'S', 'e', 'r', 'v', 'e', 'r', ' ',
		'0', '0', '1',
		0xF0, 0xEE, 0xC2, 0x4A, 0x80, 0x68, 0xBE, 0xE8, 0x2E, 0x00, 0xD0, 0xD1,
		0x02, 0x9E, 0x7E, 0x57, 0x6E, 0xEC, 0x5D, 0x2D, 0x29, 0x80, 0x6F, 0xAB,
		0x93, 0xB8, 0xE6, 0x36, 0xCF, 0xEB, 0x31, 0xAE,
	}
	hsClientPartialKey = hsClientFullKey[:30]
	hsServerPartialKey = hsServerFullKey[:36]
)

func hsMakeDigest(key []byte, src []byte, gap int) (dst []byte) {
	h := hmac.New(sha256.New, key)
	if gap <= 0 {
		h.Write(src)
	} else {
		h.Write(src[:gap])
		h.Write(src[gap+32:])
	}
	return h.Sum(nil)
}

func hsCalcDigestPos(p []byte, base int) (pos int) {
	for i := 0; i < 4; i++ {
		pos += int(p[base+i])
	}
	pos = (pos % 728) + base + 4
	return
}

func hsFindDigest(p []byte, key []byte, base int) int {
	gap := hsCalcDigestPos(p, base)
	digest := hsMakeDigest(key, p, gap)
	if bytes.Compare(p[gap:gap+32], digest) != 0 {
		return -1
	}
	return gap
}

func hsParse1(p []byte, peerkey []byte, key []byte) (ok bool, digest []byte) {
	var pos int
	if pos = hsFindDigest(p, peerkey, 772); pos == -1 {
		if pos = hsFindDigest(p, peerkey, 8); pos == -1 {
			return
		}
	}
	ok = true
	digest = hsMakeDigest(key, p[pos:pos+32], -1)
	return
}

func hsCreate01(p []byte, time uint32, ver uint32, key []byte) {
	p[0] = 3
	p1 := p[1:]
	rand.Read(p1[8:])
	pio.PutU32BE(p1[0:4], time)
	pio.PutU32BE(p1[4:8], ver)
	gap := hsCalcDigestPos(p1, 8)
	digest := hsMakeDigest(key, p1, gap)
	copy(p1[gap:], digest)
}

func hsCreate2(p []byte, key []byte) {
	rand.Read(p)
	gap := len(p) - 32
	digest := hsMakeDigest(key, p, gap)
	copy(p[gap:], digest)
}

func (conn *Conn) HandshakeClient() (err error) {
	var random [(1 + 1536*2) * 2]byte

	C0C1C2 := random[:1536*2+1]
	C0 := C0C1C2[:1]
	C0C1 := C0C1C2[:1536+1]
	C2 := C0C1C2[1536+1:]

	S0S1S2 := random[1536*2+1:]

	C0[0] = 3
	// > C0C1
	conn.Conn.SetDeadline(time.Now().Add(timeout))
	if _, err = conn.rw.Write(C0C1); err != nil {
		return
	}
	conn.Conn.SetDeadline(time.Now().Add(timeout))
	if err = conn.rw.Flush(); err != nil {
		return
	}

	// < S0S1S2
	conn.Conn.SetDeadline(time.Now().Add(timeout))
	if _, err = io.ReadFull(conn.rw, S0S1S2); err != nil {
		return
	}

	S1 := S0S1S2[1: 1536+1]
	if ver := pio.U32BE(S1[4:8]); ver != 0 {
		C2 = S1
	} else {
		C2 = S1
	}

	// > C2
	conn.Conn.SetDeadline(time.Now().Add(timeout))
	if _, err = conn.rw.Write(C2); err != nil {
		return
	}
	conn.Conn.SetDeadline(time.Time{})
	return
}
/*:客户端要向服务器发送C0,C1,C2（按序）三个chunk，服务器向客户端发送S0,S1,S2（按序）三个chunk，然后才能进行有效的信息传输*/
func (conn *Conn) HandshakeServer() (err error) {
	var random [(1 + 1536*2) * 2]byte

	C0C1C2 := random[:1536*2+1]
	/*C0是 RTMP 協定版本號碼，固定為 0x03，大小為 1 octet。 */
	C0 := C0C1C2[:1] /*only one byte ： c0和s0包是一个1字节,可以看作是一个byte*/
	/*C1是 client timestamp + 4 zeros + 1536bytes random number。*/
	C1 := C0C1C2[1: 1536+1] /*c1 is 1536 bytes :4 TIMESTAMP + 4 ZERO + RANDOM BYTES(1528 BYTES) , TIMESTAMP : 本终端发送的所有后续块的时间起点。这个值可以是 0，或者一些任意值。要同步多个块流，终端可以发送其他块流当前的 timestamp 的值*/
	C0C1 := C0C1C2[:1536+1]
	C2 := C0C1C2[1536+1:]

	S0S1S2 := random[1536*2+1:]
	S0 := S0S1S2[:1] /*1 BYTE:RTMP VERSION*/
	S1 := S0S1S2[1: 1536+1]
	S0S1 := S0S1S2[:1536+1]
	S2 := S0S1S2[1536+1:]

	// < C0C1
	conn.Conn.SetDeadline(time.Now().Add(timeout))
	if _, err = io.ReadFull(conn.rw, C0C1); err != nil {
		return
	}
	conn.Conn.SetDeadline(time.Now().Add(timeout))
	if C0[0] != 3 { /*目前rtmp版本定义为3*/
		err = fmt.Errorf("rtmp: handshake version=%d invalid", C0[0])
		return
	}

	/*S0: 是 Server 端的RTMP 協定版本號碼 */
	S0[0] = 3
	/*process C1*/
	clitime := pio.U32BE(C1[0:4]) /*时间戳:该字段占4字节,包含了一个时间戳,它是所有从这个端点发送出去的将来数据块的起始点,它可以是零,或是任意值,为了同步多个数据块流,端点可能会将这个字段设成其它数据块流时间戳的当前值.*/
	srvtime := clitime
	srvver := uint32(0x0d0e0a0d) /*SERVER VERSION*/
	cliver := pio.U32BE(C1[4:8]) /*CLIENT VERSION 0:此标记位占4字节,并且必须是0*/

	if cliver != 0 { /*IF THE 4 BYTE NOT 0 */
		var ok bool
		var digest []byte
		if ok, digest = hsParse1(C1, hsClientPartialKey, hsServerFullKey); !ok {
			err = fmt.Errorf("rtmp: handshake server: C1 invalid")
			return
		}
		hsCreate01(S0S1, srvtime, srvver, hsServerPartialKey)
		hsCreate2(S2, digest)
	} else { /*c2和s2包长都是1536字节,几乎是s1和c1的回显.*/
		copy(S1, C2) /*S1 == C2*/
		copy(S2, C1) /*S2 == C1 */
	}

	// > S0S1S2
	conn.Conn.SetDeadline(time.Now().Add(timeout))
	if _, err = conn.rw.Write(S0S1S2); err != nil {
		return
	}
	conn.Conn.SetDeadline(time.Now().Add(timeout))
	if err = conn.rw.Flush(); err != nil {
		return
	}

	// < C2
	conn.Conn.SetDeadline(time.Now().Add(timeout))
	if _, err = io.ReadFull(conn.rw, C2); err != nil {
		return
	}
	conn.Conn.SetDeadline(time.Time{})
	return
}


/* https://github.com/ossrs/srs/wiki/v1_CN_RTMPHandshake
rtmp 1.0规范中，指定了RTMP的握手协议：

【1】 简单握手
c0/s0：一个字节，说明是明文还是加密。
c1/s1: 1536字节，4字节时间，4字节0x00，1528字节随机数
c2/s2: 1536字节，4字节时间1，4字节时间2，1528随机数和s1相同。 这个就是srs以及其他开源软件所谓的simple handshake，简单握手，标准握手，FMLE也是使用这个握手协议。

【2】复杂握手 http://blog.csdn.net/win_lin/article/details/13006803
Flash播放器连接服务器时，若服务器只支持简单握手，则无法播放h264和aac的流，可能是adobe的限制。
adobe将简单握手改为了有一系列加密算法的复杂握手（complex handshake） ，详细协议分析参考变更的RTMP握手
*/