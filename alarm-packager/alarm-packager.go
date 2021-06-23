package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

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
	packager.InfoLog.Println("Saving connection settings to a file")
	err = entities.SaveCommonSettingsToFile()
	if err != nil {
		packager.ErrorLog.Fatalln("Error while saving connection settings to a file:", err.Error())
	}
	packager.Run()
}

func (packager *Packager) Run() {
	packager.InfoLog.Println("Preparing the update description")
	err := packager.fillUpdateDescription()
	if err != nil {
		packager.ErrorLog.Fatalln("Error while preparing the update description:", err.Error())
	}
	packager.InfoLog.Println("Saving the update description")
	err = packager.saveUpdateDescriptionToFile()
	if err != nil {
		packager.ErrorLog.Fatalln("Error while saving the update description:", err.Error())
	}
	packager.showFurtherActions()
}

func (packager *Packager) fillUpdateDescription() error {
	packager.UpdateDescription = entities.NewUpdateDescription()
	for key, value := range entities.AllowedUserRoles {
		packager.UpdateDescription.Roles[key] = value
	}
	for key, value := range entities.ExecutablesByUserRoles {
		packager.UpdateDescription.Executables[key] = value
	}
	for _, fileName := range entities.FilesWithChecksum {
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

func (packager *Packager) showFurtherActions() {
	filesArray := make([]string, 0, len(packager.UpdateDescription.Files)+1)
	for fileName := range packager.UpdateDescription.Files {
		filesArray = append(filesArray, fileName)
	}
	filesArray = append(filesArray, entities.VersionFileName)
	sort.Strings(filesArray)
	var builder strings.Builder
	builder.Grow(1024)
	fmt.Fprintf(&builder, "You should upload the following files to the folder %s:\n", entities.Settings.ServerUpdateFolder)
	for i, fileName := range filesArray {
		if i == 0 {
			fmt.Fprint(&builder, fileName)
		} else {
			fmt.Fprintf(&builder, ",\n%s", fileName)
		}
	}
	for userRole, filesArray := range packager.UpdateDescription.Roles {
		fmt.Fprintf(&builder,
			"\n\nFor a user with the \"%s\" role, copy the following files to the local computer:\n", userRole)
		for i, fileName := range filesArray {
			if i == 0 {
				fmt.Fprint(&builder, fileName)
			} else {
				fmt.Fprintf(&builder, ",\n%s", fileName)
			}
		}
		if userRole == "client" {
			fmt.Fprintf(&builder, "\nAt system startup, set the command to run: alarm-updater -type = %s", userRole)
		} else {
			fmt.Fprint(&builder, "\nAt system startup, set the command to run: alarm-updater")
		}
	}
	packager.InfoLog.Println(builder.String())
}
