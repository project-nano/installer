package main

import (
	"fmt"
	"path/filepath"
	"os"
	"encoding/json"
	"io/ioutil"
	"os/exec"
	"os/user"
	"bytes"
	"strings"
	"github.com/pkg/errors"
)

const (
	MonitorPortBegin   = 5901
	MonitorPortEnd     = 6000
	InitiatorMagicPort = 25469
	DHCPServerPort     = 67
)

func CellInstaller(session *SessionInfo) (ranges []PortRange, err error){
	const (
		ModulePathName    = "cell"
		ConfigPathName    = "config"
		ModuleExecuteName = "cell"
	)
	fmt.Println("installing cell module...")
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
	var configPath = filepath.Join(workingPath, ConfigPathName)
	if err = ensurePath(configPath, "config", session.UID, session.GID);err != nil{
		return
	}
	if err = enableLibvirtService(session);err != nil{
		return
	}
	if err = writeCellDomainConfig(session, configPath);err != nil{
		return
	}
	if err = installPolkitAccess(session); err != nil{
		return
	}

	ranges = []PortRange{
		{MonitorPortBegin, MonitorPortEnd, "tcp"},
		{InitiatorMagicPort, InitiatorMagicPort, "tcp"},
		{DHCPServerPort, DHCPServerPort, "udp"},
	}
	fmt.Println("cell module installed")
	return ranges, nil
}

func installCellDependencyPackages() (err error){
	const (
		PackagePath = "rpms"
		CellPath = "cell"
	)
	var packagePath = filepath.Join(PackagePath, CellPath)
	if _, err = os.Stat(packagePath); os.IsNotExist(err){
		err = fmt.Errorf("can not find dependency package path %s", packagePath)
		return
	}
	fmt.Println("installing cell dependency packages...")
	var cmd = exec.Command("rpm", "-i", "--force", fmt.Sprintf("%s/*", packagePath))
	var errOutput bytes.Buffer
	cmd.Stderr = &errOutput
	if err = cmd.Run();err != nil{
		fmt.Printf("install pacakge fail: %s, %s\n", err.Error(), errOutput.String())
		fmt.Println("try installing from online reciprocity...")
		{
			//install EPEL first
			epel := exec.Command("yum", "install", "-y", "epel-release")
			epel.Run()
		}
		cmd = exec.Command("yum", "install", "-y", "qemu-system-x86", "bridge-utils","libvirt",
			"seabios", "genisoimage", "nfs-utils", "policycoreutils-python")
		if err = cmd.Run();err != nil {
			fmt.Printf("install online reciprocity fail: %s, %s\n", err.Error(), errOutput.String())
			return
		}
	}
	fmt.Println("dependency packages installed")
	return nil
}

func enableLibvirtService(session *SessionInfo) (err error){
	if err = configureLibvirtGroup(session);err != nil{
		return
	}
	{
		var cmd = exec.Command("systemctl", "enable", "libvirtd")
		if err = cmd.Run();err != nil{
			fmt.Printf("enable libvirt fail: %s\n", err.Error())
			return
		}else{
			fmt.Println("libvirt enabled")
		}
	}
	{
		var cmd = exec.Command("systemctl", "start", "libvirtd")
		if err = cmd.Run();err != nil{
			fmt.Printf("start libvirt fail: %s\n", err.Error())
			return
		}else{
			fmt.Println("libvirt started")
		}
	}

	return
}
func writeCellDomainConfig(session *SessionInfo, configPath string) (err error){
	const (
		DomainConfigFileName = "domain.cfg"
	)
	type DomainConfig struct {
		Domain        string `json:"domain"`
		GroupAddress  string `json:"group_address"`
		GroupPort     int    `json:"group_port"`
	}

	var configFile = filepath.Join(configPath, DomainConfigFileName)
	if _, err = os.Stat(configFile); os.IsNotExist(err) {
		var config = DomainConfig{Domain:session.Domain, GroupAddress:session.GroupAddress, GroupPort:session.GroupPort}
		//write
		var data []byte
		data, err = json.MarshalIndent(config, "", " ")
		if err != nil {
			return
		}
		if err = ioutil.WriteFile(configFile, data, DefaultFilePerm); err != nil {
			return
		}
		fmt.Printf("domain configure '%s' generated\n", configFile)
	}
	return nil
}

func installPolkitAccess(session *SessionInfo) (err error){
	const (
		FileName = "/etc/polkit-1/localauthority/50-local.d/50-org.libvirt-group-access.pkla"
	)
	if _, err = os.Stat(FileName);os.IsNotExist(err){
		//need install
		var file *os.File
		file, err = os.Create(FileName)
		if err != nil{
			return err
		}
		fmt.Fprintln(file, "[libvirt group Management Access]")
		fmt.Fprintln(file, "Identity=unix-group:libvirt")
		fmt.Fprintln(file, "Action=org.libvirt.unix.manage")
		fmt.Fprintln(file, "ResultAny=yes")
		fmt.Fprintln(file, "ResultInactive=yes")
		fmt.Fprintln(file, "ResultActive=yes")
		file.Close()
		fmt.Println("polkit access installed")

	}else{
		fmt.Println("polkit access alreay installed")
	}
	return nil
}

func configureLibvirtGroup(session *SessionInfo) (err error) {
	const (
		GroupName = "libvirt"
	)
	if err = enableQEMUAuthority(session.User, session.UserGroup); err != nil{
		return err
	}
	if _, err = user.LookupGroup(GroupName);err != nil{
		var cmd = exec.Command("groupadd","libvirt")
		if err = cmd.Run();err != nil{
			fmt.Printf("create group fail: %s\n", err.Error())
			return
		}else{
			fmt.Printf("new group %s created", GroupName)
		}
	}else{
		fmt.Printf("group %s already exists\n", GroupName)
	}
	libvirtGroup, err := user.LookupGroup(GroupName)
	if err != nil{
		fmt.Printf("get group %s fail: %s", GroupName, err.Error())
		return
	}
	currentUser, err := user.Lookup(session.User)
	if err != nil{
		fmt.Printf("get current user fail: %s", err.Error())
		return
	}
	groups, err := currentUser.GroupIds()
	if err != nil{
		fmt.Printf("get groups for user %s fail: %s", session.User, err.Error())
		return
	}

	for _, groupID := range groups{
		if groupID == libvirtGroup.Gid{
			fmt.Printf("user %s already in group %s", session.User, GroupName)
			return nil
		}
	}
	//need add
	var cmd = exec.Command("usermod","-a", "-G", GroupName, session.User)
	if err = cmd.Run();err != nil{
		fmt.Printf("add %s to group %s fail: %s\n", session.User, GroupName, err.Error())
		return
	}else{
		fmt.Printf("user %s added to group %s\n", session.User, GroupName)
	}
	return nil
}

func enableQEMUAuthority(user, group string) (err error){
	const (
		ConfigPath = "/etc/libvirt/qemu.conf"
		DefaultUser = "#user = \"root\""
		DefaultGroup = "#group = \"root\""
		KVMDevice = "/dev/kvm"
	)
	data, err := ioutil.ReadFile(ConfigPath)
	if err != nil{
		return err
	}
	var userString = fmt.Sprintf("user = \"%s\"", user)
	var groupString = fmt.Sprintf("group = \"%s\"", group)
	var content = strings.Replace(string(data), DefaultUser, userString, 1)
	content = strings.Replace(content, DefaultGroup, groupString, 1)
	if err = ioutil.WriteFile(ConfigPath, []byte(content), DefaultFilePerm);err != nil{
		return
	}
	fmt.Printf("user %s / group %s updated in %s\n", user, group, ConfigPath)
	{
		if _, err = os.Stat(KVMDevice); os.IsNotExist(err){
			err = errors.New("No KVM module available, check Intel VT-x/AMD-v in BIOS to enable virtualization before installing Nano")
			return
		}
		var cmd = exec.Command("chown", fmt.Sprintf("%s:%s", user, group), KVMDevice)
		if err = cmd.Run(); err != nil{
			return
		}
		fmt.Printf("%s owner changed\n", KVMDevice)
	}
	return nil
}