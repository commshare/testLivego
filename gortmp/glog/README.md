[glog](https://raw.githubusercontent.com/sdbaiguanghe/glog/master/README.md)
====

https://supereagle.github.io/2017/06/07/golang-glog/
====

在[golang/glog](https://github.com/golang/glog)的基础上做了一些修改。

## 修改的地方:
1. 增加每天切割日志文件的功能,程序运行时指定 --dailyRolling=true参数即可
2. 将日志等级由原来的INFO WARN ERROR FATAL改为DEBUG INFO ERROR FATAL
3. 增加日志输出等级设置,当日志信息等级低于输出等级时则不输出日志信息
4. 将默认的刷新缓冲区时间由20s改为5s

##使用示例 
```
func main() {
    //初始化命令行参数
    flag.Parse()
    //退出时调用，确保日志写入文件中
    defer glog.Flush()
    
    //一般在测试环境下设置输出等级为DEBUG，线上环境设置为INFO
    glog.SetLevelString("DEBUG") 
    
    glog.Info("hello, glog")
    glog.Warning("warning glog")
    glog.Error("error glog")
    
    glog.Infof("info %d", 1)
    glog.Warningf("warning %d", 2)
    glog.Errorf("error %d", 3)
 }
 
//假设编译后的可执行程序名为demo,运行时指定log_dir参数将日志文件保存到特定的目录
// ./demo --log_dir=./log --dailyRolling=true 
```

##
Log files输出到默认的临时目录（C:\Users\robin\AppData\Local\Temp）下。
在标准输出上只能看到Error和Fatal log，是因为默认的--stderrthreshold为ERROR。
不过所有log都会输出到log files，除非设置--logtostderr=true。
设置--alsologtostderr=true，所有log除了输出到log files，还会输出到标准输出，此选项会覆盖--stderrthreshold的设置。

需要注意的是，调用glog.Fatal("Fatal")，程序会输出所有goroutine的堆栈信息，然后调用os.Exit()退出程序。
所以，其前面的defer代码以及后面的代码，都不会执行