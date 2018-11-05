#

##
- publish flv 的时候，需要用到这个，dst设置为一个conn就ok
- demuxer是从容器读入数据的
```$xslt
// Demuxer can read compressed audio/video packets from container formats like MP4/FLV/MPEG-TS.
type Demuxer interface {
	PacketReader // read compressed audio/video packets
	Streams() ([]CodecData, error) // reads the file header, contains video/audio meta infomations
}

```


```$xslt

func CopyFile(dst av.Muxer, src av.Demuxer) (err error) {
	var streams []av.CodecData
	if streams, err = src.Streams(); err != nil {
		return
	}
	if err = dst.WriteHeader(streams); err != nil {
		return
	}
	if err = CopyPackets(dst, src); err != nil {
		return
	}
	if err = dst.WriteTrailer(); err != nil {
		return
	}
	return
}

```

- 看起来，CopyPackets 就是从src不断读的同时不断的写，一个一个的读， 然后一个一个的写