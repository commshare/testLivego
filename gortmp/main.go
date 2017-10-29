package main

import (
	//"./mpegts"
	"./rtmp"
	//"fmt"
	//"os"
	//"./avformat"
	"./config"
	"fmt"
	"./glog"
	"flag"
)

func main() {
	var err error
    fmt.Println("------main---------")
	/* https://github.com/cloudnativelabs/kube-router/commit/78a0aeb39793c86a7fcb9688bf63a72a6cbb4f90
// Workaround for this issue:
+	// https://github.com/kubernetes/kubernetes/issues/17162
*/
	//flag.CommandLine.Parse([]string{})
	flag.Parse()
	glog.SetLevelString("DEBUG")

	glog.Info("---test glog---")

	if err = InitAppConfig(); err != nil {
		return
	}

	glog.Infof("Shutting down network policies controller")
	fmt.Println("why glog not output ?  ./main --alsologtostderr=true")
	l := ":1935"
	err = rtmp.ListenAndServe(l)
	if err != nil {
		panic(err)
	}

	select {}

}

func InitAppConfig() (err error) {
	cfg := new(config.Config)
	err = cfg.Init("app.conf")
	if err != nil {
		return
	}

	return
}
