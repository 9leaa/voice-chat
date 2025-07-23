package main

import (
	"flag"
	"log"
)

func main() {
	port := flag.String("port", "8080", "服务端口")
	flag.Parse()
	log.Println("启动语音信令服务器...")
	StartSignalingServer(*port)
}
