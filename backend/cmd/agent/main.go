package main

import (
	"log"
	"os"

	"pbmanager/backend/internal/agent"
)

func main() {
	if os.Geteuid() != 0 {
		log.Fatal("the provisioning agent must run as root")
	}
	if err := agent.Run(); err != nil {
		log.Fatal(err)
	}
}
