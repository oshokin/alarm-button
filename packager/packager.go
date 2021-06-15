package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"os"

	"github.com/oshokin/alarm-button/entities"
	"gopkg.in/yaml.v3"
)

type Packager struct {
	UpdateDescription *entities.UpdateDescription
	InfoLog           *log.Logger
	ErrorLog          *log.Logger
}

func NewPackager() (*Packager, error) {
	packager := Packager{
		UpdateDescription: nil,
		InfoLog:           log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime),
		ErrorLog:          log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile),
	}
	return &packager, nil
}

func main() {
	packager, err := NewPackager()
	if err != nil {
		packager.ErrorLog.Fatalln("Ошибка при запуске упаковщика:", err.Error())
	}
	packager.Run()
}

func (packager *Packager) Run() {
	err := packager.fillUpdateDescription()
	if err != nil {
		packager.ErrorLog.Fatalln("Ошибка при подготовке описания обновления:", err.Error())
	}
	err = packager.saveUpdateDescriptionToFile()
	if err != nil {
		packager.ErrorLog.Fatalln("Ошибка при сохранении описания обновления:", err.Error())
	}
}

func (packager *Packager) fillUpdateDescription() error {
	packager.UpdateDescription = entities.NewUpdateDescription()
	allFiles := []string{"button-off.exe", "button-on.exe", "checker.exe", "server.exe", "updater.exe"}
	for _, fileName := range allFiles {
		if _, err := os.Stat(fileName); os.IsNotExist(err) {
			return fmt.Errorf(fmt.Sprintf("%s не найден", fileName))
		}
		fileChecksum, err := entities.GetFileChecksum(fileName)
		if err != nil {
			return err
		}
		packager.UpdateDescription.Files[fileName] = base64.StdEncoding.EncodeToString(fileChecksum)
	}
	packager.UpdateDescription.Roles["user"] = []string{"button-on.exe", "checker.exe", "updater.exe"}
	packager.UpdateDescription.Roles["advanced-user"] = []string{"button-off.exe", "button-on.exe", "checker.exe", "updater.exe"}
	packager.UpdateDescription.Roles["server"] = allFiles

	return nil
}

func (packager *Packager) saveUpdateDescriptionToFile() error {
	contents, err := yaml.Marshal(packager.UpdateDescription)
	if err != nil {
		return err
	}
	err = os.WriteFile(entities.VersionFileName, contents, entities.DefaultFileMode)
	if err != nil {
		return err
	}

	return nil
}
