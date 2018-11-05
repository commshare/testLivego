
#

##
- ffmpeg 会尽可能快的读取输入
- re 以native的帧率读入，用于模拟从文件中读取时的采集设备或者直播输入流
- 如果使用实际的采集设备或者直播流，这个选项不可以用，因为会导致丢包
```

-re (input) 

Read input at native frame rate. Mainly used to simulate a grab device, or live input stream
 (e.g. when reading from a file).
  Should not be used with actual grab devices or live input streams (where it can cause packet loss). 
  By default ffmpeg attempts to read the input(s) as fast as possible. This option will slow down the reading of the input(s) 
  to the native frame rate of the input(s). 
It is useful for real-time output (e.g. live streaming).

```