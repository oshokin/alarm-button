package main

import (
	"github.com/oshokin/alarm-button/entities"
)

func main() {
	client, err := entities.NewClient()
	if err != nil {
		client.ErrorLog.Println("Ошибка при запуске клиента:", err.Error())
		client.Stop(false, 1)
	}
	client.RunChecker()
}
