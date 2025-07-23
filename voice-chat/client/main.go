package main

import (
	"log"
	"os"
	"os/signal"
	"runtime/debug"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("程序崩溃: %v\n%s", r, debug.Stack())
		}
	}()
	client := NewUIClient()
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		log.Println("正在关闭应用...")
		os.Exit(0)
	}()
	client.Start()
}
