
#

##这是一个buf类
- 可以扩充，扩充的时候要复制原有数据到新slice里
- packet 放在一个slice里，叫做pkt，默认预分配64个位置，其他默认都是0
- 有tail和head游标指向，游标类型目前是int
- buf的数据量大小是size ，是pkt实际数据量的总和
- buf的count是记录slice里实际有包的数目
- push的时候，检测count是否与pkt slice预分配大小一致，一致则grow，每次grow，count++
- head和tail 一直递增，而又是用来pop和push计算index的，所以这里有一个技巧？

```
package pktque

import (
	"github.com/nareix/joy4/av"
)

type Buf struct {
	Head, Tail BufPos
	pkts       []av.Packet
	Size       int
	Count      int
}

func NewBuf() *Buf {
	return &Buf{
		pkts: make([]av.Packet, 64),
	}
}

func (self *Buf) Pop() av.Packet {
	if self.Count == 0 {
		panic("pktque.Buf: Pop() when count == 0")
	}

	i := int(self.Head) & (len(self.pkts) - 1)
	pkt := self.pkts[i]
	self.pkts[i] = av.Packet{}
	self.Size -= len(pkt.Data)
	self.Head++
	self.Count--

	return pkt
}

func (self *Buf) grow() {
	newpkts := make([]av.Packet, len(self.pkts)*2)
	for i := self.Head; i.LT(self.Tail); i++ {
		newpkts[int(i)&(len(newpkts)-1)] = self.pkts[int(i)&(len(self.pkts)-1)]
	}
	self.pkts = newpkts
}

func (self *Buf) Push(pkt av.Packet) {
	if self.Count == len(self.pkts) {
		self.grow()
	}
	self.pkts[int(self.Tail)&(len(self.pkts)-1)] = pkt
	self.Tail++
	self.Count++
	self.Size += len(pkt.Data)
}

func (self *Buf) Get(pos BufPos) av.Packet {
	return self.pkts[int(pos)&(len(self.pkts)-1)]
}

func (self *Buf) IsValidPos(pos BufPos) bool {
	return pos.GE(self.Head) && pos.LT(self.Tail)
}

type BufPos int

func (self BufPos) LT(pos BufPos) bool {
	return self-pos < 0
}

func (self BufPos) GE(pos BufPos) bool {
	return self-pos >= 0
}

func (self BufPos) GT(pos BufPos) bool {
	return self-pos > 0
}
```

## BufPos的这种用法
- 这好像是在实现运算符重载一样的？
- 好像是声明了一个类型，并做了一些运算

```
type BufPos int

func (self BufPos) LT(pos BufPos) bool {
	return self-pos < 0
}

func (self BufPos) GE(pos BufPos) bool {
	return self-pos >= 0
}

func (self BufPos) GT(pos BufPos) bool {
	return self-pos > 0
}
```

## 确认的问题
###1 slice是引用类型么？我看pkt没用用指针

###2 tail和head是怎么用的？设计他们有什么意义？
- 初始化的时候，都是0
- push的时候，tail +1 
- pop的时候 head +1 
- tail始终指向新插入的元素位置
-  head和tail是插入和删除的时候用的索引 ！！！！而且他俩一直都是递增的，这个还真是要测试一下。


## 技巧

###  计算index用了& 确保一定正确？
- pop 的位置
```
	i := int(self.Head) & (len(self.pkts) - 1)

```

- push的位置
```$xslt

int(self.Tail)&(len(self.pkts)-1)
```