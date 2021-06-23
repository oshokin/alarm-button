package main

import (
	"encoding/base64"
	"errors"
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
	isUpdaterRunningNow := entities.IsUpdaterRunningNow(packager.InfoLog, packager.ErrorLog)
	if isUpdaterRunningNow {
		return &packager, errors.New("the updater is running now")
	}
	err := entities.ReadCommonSettingsFromArgs()
	return &packager, err
}

func main() {
	packager, err := NewPackager()
	if err != nil {
		packager.ErrorLog.Fatalln("Error while launching packager:", err.Error())
	}
	err = entities.SaveCommonSettingsToFile()
	if err != nil {
		packager.ErrorLog.Fatalln("Error while saving settings to a file:", err.Error())
	}
	packager.Run()
}

func (packager *Packager) Run() {
	err := packager.fillUpdateDescription()
	if err != nil {
		packager.ErrorLog.Fatalln("Error while preparing update description:", err.Error())
	}
	err = packager.saveUpdateDescriptionToFile()
	if err != nil {
		packager.ErrorLog.Fatalln("Error while saving update description:", err.Error())
	}
}

func (packager *Packager) fillUpdateDescription() error {
	packager.UpdateDescription = entities.NewUpdateDescription()
	for key, value := range entities.AllowedUserRoles {
		packager.UpdateDescription.Roles[key] = value
	}
	for key, value := range entities.ExecutablesByUserRoles {
		packager.UpdateDescription.Executables[key] = value
	}
	for _, fileName := range entities.AllExecutableFiles {
		if _, err := os.Stat(fileName); os.IsNotExist(err) {
			return fmt.Errorf(fmt.Sprintf("%s wasn't found", fileName))
		}
		fileChecksum, err := entities.GetFileChecksum(fileName)
		if err != nil {
			return err
		}
		packager.UpdateDescription.Files[fileName] = base64.StdEncoding.EncodeToString(fileChecksum)
	}
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
