package main

import (
	"nano/framework"
	"fmt"
	"os"
	"path/filepath"
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
	panic("not implement")
}
