package main

import (
	"github.com/oshokin/alarm-button/entities"
)

func main() {
	client, err := entities.NewClient()
	if err != nil {
		client.ErrorLog.Println("Error while starting client:", err.Error())
		client.Stop(false, 1)
	}
	client.RunAlarmer(true)
}
