package main

import (
	"github.com/project-nano/framework"
	"fmt"
	"os"
	"path/filepath"
	"path"
	"os/exec"
	"bytes"
	"strings"
	"github.com/pkg/errors"
	"crypto/sha1"
	"io"
	"time"
)
type ResourcePath struct {
	Source string
	Target string
}

type ModuleBinary struct {
	Module string
	Binary string
	Resources []ResourcePath
}

func UpdateAllModules() {
	var modules = map[string]ModuleBinary{
		"core": ModuleBinary{"core", "core", nil},
		"cell": ModuleBinary{"cell", "cell", nil},
		"frontend": {"frontend", "frontend", []ResourcePath{{path.Join("bin", FrontEndFilesPath, FrontEndWebPath), FrontEndWebPath}}},
	}

	var moduleOrder = []string{"core", "cell", "frontend"}

	const (
		DefaultProjectPath = "/opt/nano"
	)
	var projectPath string
	var err error
	projectPath, err = framework.InputString("Project Installed Path", DefaultProjectPath)
	if err != nil{
		fmt.Printf("get installed path fail: %s\n", err.Error())
		return
	}

	if _, err = os.Stat(projectPath); os.IsNotExist(err){
		fmt.Printf("project path '%s' not exists\n", projectPath)
		return
	}

	var binaries []ModuleBinary

	for _, moduleName := range moduleOrder{
		if _, err = os.Stat(filepath.Join(projectPath, moduleName)); os.IsNotExist(err){
			continue
		}else if binary, exists := modules[moduleName]; exists{
			binaries = append(binaries, binary)
		}else{
			fmt.Printf("invalid module '%s' in path '%s'\n", moduleName, projectPath)
			return
		}
	}

	if 0 == len(binaries){
		fmt.Println("no module binary available")
		return
	}
	for _, binary := range binaries {
		err = updateModule(projectPath, binary)
		if err != nil{
			fmt.Printf("update module '%s' fail: %s\n", binary.Module, err.Error())
			return
		}
	}
	fmt.Printf("%d module(s) updated success\n", len(binaries))
}

func updateModule(projectPath string, binary ModuleBinary) (err error) {
	var sourceBinary = path.Join(BinaryPathName, binary.Binary)
	var binaryName = path.Join(projectPath, binary.Module, binary.Binary)
	isIdentical, err := isIdentical(sourceBinary, binaryName)
	if err != nil{
		return err
	}
	if isIdentical{
		fmt.Printf("module %s already updated\n", binary.Module)
		return nil
	}

	isRunning, err := isModuleRunning(binaryName)
	if err != nil{
		return err
	}
	if isRunning{
		//stop first
		if err = stopModule(binaryName); err != nil{
			err = fmt.Errorf("stop binary '%s' fail: %s\n", binaryName, err.Error())
			return
		}
		fmt.Printf("module %s stopped\n", binary.Module)
		const (
			StopGap = time.Millisecond * 300
		)
		time.Sleep(StopGap)
	}

	if err = copyFile(sourceBinary, binaryName); err != nil{
		err = fmt.Errorf("overwrite binary '%s' fail: %s\n", binaryName, err.Error())
		return
	}
	fmt.Printf("overwrite %s success\n", binaryName)
	if 0 != len(binary.Resources){
		for _, resource := range binary.Resources{
			var targetPath = path.Join(projectPath, binary.Module, resource.Target)
			if err = copyDir(resource.Source, targetPath); err != nil{
				err = fmt.Errorf("overwrite to resource path '%s' fail: %s\n", targetPath, err.Error())
				return
			}
			fmt.Printf("resoure %s overwritten to '%s'\n", resource.Source, targetPath)
		}
	}

	if isRunning{
		//start again
		if err = startModule(binaryName); err != nil{
			err = fmt.Errorf("restart binary '%s' fail: %s\n", binaryName, err.Error())
			return
		}
		fmt.Printf("module %s restarted\n", binary.Module)
	}
	fmt.Printf("module %s update success\n", binary.Module)
	return nil
}

func isIdentical(source, target string) (identical bool, err error){
	var files = []string{target, source}
	var hashResult [][]byte
	for _, filename := range files{
		var fileStream *os.File
		fileStream, err = os.Open(filename)
		if err != nil{
			return
		}
		defer fileStream.Close()
		var hashLoader = sha1.New()
		if _, err = io.Copy(hashLoader, fileStream); err != nil{
			return
		}
		var fileHash = hashLoader.Sum(nil)
		hashResult = append(hashResult, fileHash)
	}
	const (
		TargetHashOffset = iota
		SourceHashOffset
		ValidHashCount   = 2
	)
	if ValidHashCount != len(hashResult){
		err = fmt.Errorf("invalid hash count %d", len(hashResult))
		return false, err
	}

	return bytes.Equal(hashResult[TargetHashOffset], hashResult[SourceHashOffset]), nil
}

func isModuleRunning(binaryPath string) (running bool, err error) {
	var cmd = exec.Command(binaryPath, "status")
	var output bytes.Buffer
	cmd.Stdout = &output
	if err = cmd.Run(); err != nil{
		return
	}
	const (
		Keyword = "running"
	)
	return strings.Contains(output.String(), Keyword), nil
}

func startModule(binaryPath string) (err error){
	var cmd = exec.Command(binaryPath, "start")
	var output bytes.Buffer
	cmd.Stdout = &output
	if err = cmd.Run(); err != nil{
		return
	}
	const (
		Keyword = "fail"
	)
	var content = output.String()
	if strings.Contains(content, Keyword){
		//fail
		return errors.New(content)
	}
	return nil
}

func stopModule(binaryPath string) (err error){
	var cmd = exec.Command(binaryPath, "stop")
	var output bytes.Buffer
	cmd.Stdout = &output
	if err = cmd.Run(); err != nil{
		return
	}
	const (
		Keyword = "fail"
	)
	var content = output.String()
	if strings.Contains(content, Keyword){
		//fail
		return errors.New(content)
	}
	return nil
}
