package main

import (
	"path/filepath"
	"os"
	"fmt"
	"nano/framework"
	"encoding/json"
	"io/ioutil"
)

const (
	PortalPortBegin   = 5870
	PortalPortEnd     = 5899
)

func FrontendInstaller(session *SessionInfo) (ranges []PortRange, err error){
	const (
		ModulePathName    = "frontend"
		ConfigPathName    = "config"
		ModuleExecuteName = "frontend"
	)
	fmt.Println("installing frontend module...")
	var workingPath = filepath.Join(session.ProjectPath, ModulePathName)
	if err = ensurePath(workingPath, "module", session.UID, session.GID);err != nil{
		return
	}
	var sourceFile = filepath.Join(session.BinaryPath, ModuleExecuteName)
	if _, err = os.Stat(sourceFile); os.IsNotExist(err){
		return
	}
	var targetFile = filepath.Join(workingPath, ModuleExecuteName)
	if err = copyFile(sourceFile, targetFile);err != nil{
		return
	}
	if err = enableExecuteAccess(session, targetFile);err != nil{
		fmt.Printf("enable execute access fail: %s\n", err.Error())
		return
	}
	fmt.Printf("binary '%s' copied\n", targetFile)
	if err = copyResources(session, workingPath); err != nil{
		return
	}
	var configPath = filepath.Join(workingPath, ConfigPathName)
	if err = ensurePath(configPath, "config", session.UID, session.GID);err != nil{
		return
	}
	if err = writeFrontEndConfig(session, configPath);err != nil{
		return
	}
	ranges = []PortRange{{PortalPortBegin, PortalPortEnd, "tcp"}}
	fmt.Println("frontend module installed")
	return ranges, nil
}

func copyResources(session *SessionInfo, workingPath string) (err error){
	const (
		FilesPath = "frontend_files"
		ResourcePathName = "resource"
	)
	var sourcePath = filepath.Join(session.BinaryPath, FilesPath, ResourcePathName)
	if _, err = os.Stat(sourcePath);os.IsNotExist(err){
		return
	}
	var targetPath = filepath.Join(workingPath, ResourcePathName)
	return copyDir(sourcePath, targetPath)
}

func writeFrontEndConfig(session *SessionInfo, configPath string) (err error){
	const (
		ConfigFileName      = "frontend.cfg"
		DefaultBackEndPort  = 5850
		DefaultFrontEndPort = 5870
	)
	type FrontEndConfig struct {
		ListenAddress string `json:"address"`
		ListenPort    int    `json:"port"`
		ServiceHost   string `json:"service_host"`
		ServicePort   int    `json:"service_port"`
	}

	var configFile = filepath.Join(configPath, ConfigFileName)
	if _, err = os.Stat(configFile); os.IsNotExist(err) {
		fmt.Println("No configures available, following instructions to generate a new one.")

		var config = FrontEndConfig{}
		if session.LocalAddress != ""{
			config.ListenAddress = session.LocalAddress
			fmt.Printf("using %s as portal listen address\n", session.LocalAddress)
		}else{
			config.ListenAddress, err = framework.ChooseIPV4Address("Portal listen address")
			if err != nil{
				return
			}
			session.LocalAddress = config.ListenAddress
		}
		if config.ListenPort, err = framework.InputNetworkPort(fmt.Sprintf("Portal listen port (%d ~ %d)", PortalPortBegin, PortalPortEnd),
			DefaultFrontEndPort); err !=nil{
			return
		}
		if session.APIAddress != ""{
			//same host
			config.ServiceHost = session.APIAddress
			fmt.Printf("using %s as api address\n", session.APIAddress)
		}else{
			if config.ServiceHost, err = framework.InputIPAddress("Backend API Host Address", config.ListenAddress); err !=nil{
				return
			}
			session.APIAddress = config.ServiceHost
		}

		if 0 != session.APIPort{
			config.ServicePort = session.APIPort
			fmt.Printf("using %d as backend api port\n", session.APIPort)
		}else{
			if config.ServicePort, err = framework.InputNetworkPort("Backend API port", DefaultBackEndPort); err != nil{
				return
			}
			session.APIPort = config.ServicePort
		}

		//write
		data, err := json.MarshalIndent(config, "", " ")
		if err != nil {
			return err
		}
		if err = ioutil.WriteFile(configFile, data, DefaultFilePerm); err != nil {
			return err
		}
		fmt.Printf("default configure '%s' generated\n", configFile)
	}
	return
}
