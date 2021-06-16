package main

import (
	"errors"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/oshokin/alarm-button/entities"
)

type Launcher struct {
	InfoLog  *log.Logger
	ErrorLog *log.Logger
}

func NewLauncher() (*Launcher, error) {
	launcher := Launcher{
		InfoLog:  log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime),
		ErrorLog: log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile),
	}
	if len(os.Args) < 2 {
		return &launcher, errors.New("укажите программу для запуска")
	}
	return &launcher, nil
}

func main() {
	launcher, err := NewLauncher()
	if err != nil {
		launcher.ErrorLog.Fatalln("Ошибка при запуске:", err.Error())
	}
	launcher.Run()
}

func (launcher *Launcher) Run() {
	for {
		launcher.InfoLog.Println("Проверяю наличие маркера обновления")
		fileInfo, err := os.Stat(entities.UpdateMarkerFileName)
		if err != nil {
			if os.IsNotExist(err) {
				launcher.InfoLog.Println("Маркер не найден, запускаю программу")
				err = launcher.executeCommand()
				if err != nil {
					launcher.ErrorLog.Fatalln("Ошибка при запуске:", err.Error())
				}
				break
			}
		} else {
			if time.Since(fileInfo.ModTime()) > entities.UpdateMarkerLifeTime {
				launcher.InfoLog.Println("Маркер обновления слишком старый, возможно обновление зависло. Пробую удалить файл")
				os.Remove(entities.UpdateMarkerFileName)
			}
		}
		time.Sleep(entities.LauncherSleepTime)
	}
}

func (launcher *Launcher) executeCommand() error {
	head := os.Args[1]
	args := os.Args[2:len(os.Args)]
	cmd := exec.Command(head, args...)
	err := cmd.Run()

	return err
}
