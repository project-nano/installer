package main

import (
	"nano/framework"
	"fmt"
	"os"
	"path/filepath"
	"path"
	"os/exec"
	"bytes"
	"strings"
	"github.com/pkg/errors"
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
		"frontend": {"frontend", "frontend", []ResourcePath{{"bin/frontend_files", "resource"}}},
	}

	var moduleOrder = []string{"core", "cell", "frontend"}

	const (
		DefaultProjectPath = "/opt/nano"
	)
	var projectPath string
	var err error
	projectPath, err = framework.InputString("Project Installed Path", DefaultProjectPath)
	if err != nil{
		fmt.Printf("get installed path fail: %s", err.Error())
		return
	}

	if _, err = os.Stat(projectPath); os.IsNotExist(err){
		fmt.Printf("project path '%s' not exists", projectPath)
		return
	}

	var binaries []ModuleBinary

	for _, moduleName := range moduleOrder{
		if _, err = os.Stat(filepath.Join(projectPath, moduleName)); os.IsNotExist(err){
			continue
		}else if binary, exists := modules[moduleName]; exists{
			binaries = append(binaries, binary)
		}else{
			fmt.Printf("invalid module '%s' in path '%s'", moduleName, projectPath)
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
			fmt.Printf("update module '%s' fail: %s", binary.Module, err.Error())
			return
		}
	}
	fmt.Printf("%d module(s) updated success", len(binaries))
}

func updateModule(projectPath string, binary ModuleBinary) (err error) {
	var binaryName = path.Join(projectPath, binary.Module, binary.Binary)
	isRunning, err := isModuleRunning(binaryName)
	if err != nil{
		return err
	}
	if isRunning{
		//stop first
		if err = stopModule(binaryName); err != nil{
			err = fmt.Errorf("stop binary '%s' fail: %s", binaryName, err.Error())
			return
		}
		fmt.Printf("module %s stopped", binary.Module)
	}
	var sourceBinary = path.Join(BinaryPathName, binary.Binary)
	if err = copyFile(sourceBinary, binaryName); err != nil{
		err = fmt.Errorf("overwrite binary '%s' fail: %s", binaryName, err.Error())
		return
	}
	fmt.Printf("overwrite %s success", binaryName)
	if 0 != len(binary.Resources){
		for _, resource := range binary.Resources{
			var targetPath = path.Join(projectPath, binary.Module, resource.Target)
			if err = copyDir(resource.Source, targetPath); err != nil{
				err = fmt.Errorf("overwrite to resource path '%s' fail: %s", targetPath, err.Error())
				return
			}
			fmt.Printf("resoure %s overwritten to '%s'", resource.Source, targetPath)
		}
	}

	if isRunning{
		//start again
		if err = startModule(binaryName); err != nil{
			err = fmt.Errorf("restart binary '%s' fail: %s", binaryName, err.Error())
			return
		}
		fmt.Printf("module %s restarted", binary.Module)
	}
	fmt.Printf("module %s update success", binary.Module)
	return nil
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
