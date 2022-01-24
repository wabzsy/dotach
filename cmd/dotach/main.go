package main

import (
	"dotach"
	"flag"
	"log"
	"os"
)

func main() {

	pid := flag.Int("p", 0, "target pid")
	flag.Parse()

	if *pid == 0 {
		flag.Usage()
		return
	}
	target, err := os.FindProcess(*pid)
	if err != nil {
		panic(err)
	}

	d, err := dotach.New(target)
	if err == nil {
		if err := d.Run(); err != nil {
			panic(err)
		}
	} else {
		log.Println("Error:", err)
	}

}
