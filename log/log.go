package log

import "log"

var Debug = false

func init() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
}

func Debugln(v ...interface{}) {
	if Debug {
		log.Println(v...)
	}
}

func Infoln(v ...interface{}) {
	log.Println(v...)
}

func Debugf(format string, v ...interface{}) {
	if Debug {
		log.Printf(format, v...)
	}
}

func Fatalln(v ...interface{}) {
	log.Fatalln(v...)
}

func Fatalf(format string, v ...interface{}) {
	log.Fatalf(format, v...)
}

func Errorln(v ...interface{}) {
	log.Println(v...)
}

func Errorf(format string, v ...interface{}) {
	log.Printf(format, v...)
}
