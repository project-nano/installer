package main

import (
	"fmt"
	"strings"
	"strconv"
	"path/filepath"
	"os"
	"crypto/x509"
	"crypto/x509/pkix"
	"time"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"io"
	"os/exec"
	"math/big"
	"nano/framework"
	"nano/sonar"
	"os/user"
	"io/ioutil"
	"path"
	"bufio"
	"github.com/vishvananda/netlink"
	"errors"
)

type SessionInfo struct {
	Local        bool
	Host         string
	User         string
	Password     string
	ProjectPath  string
	BinaryPath   string
	CACertPath   string
	CAKeyPath    string
	Domain       string
	GroupAddress string
	GroupPort    int
	LocalAddress string
	APIAddress   string
	APIPort      int
	UserGroup    string
	UID          int
	GID          int
}

type PortRange struct {
	Begin    int
	End      int
	Protocol string
}

//todo: add dependency params
type ModuleInstaller func(*SessionInfo) ([]PortRange, error)

const (
	ModuleCore     = iota
	ModuleFrontEnd
	ModuleCell
	ModuleExit
)

const (
	ProjectName       = "nano"
	BinaryPathName    = "bin"
	DefaultPathPerm   = 0740
	DefaultFilePerm   = 0640
	DefaultBridgeName = "br0"
	CurrentVersion    = "0.1.9"
)

func main() {
	var optionNames = map[int]string{
		ModuleCore:     "Core",
		ModuleFrontEnd: "FrontEnd",
		ModuleCell:     "Cell",
		ModuleExit:     "Exit",
	}
	var optionFunctions = map[int]ModuleInstaller{
		ModuleCore:     CoreInstaller,
		ModuleFrontEnd: FrontendInstaller,
		ModuleCell:     CellInstaller,
	}
	fmt.Printf("Welcome to %s installer v%s\n", ProjectName, CurrentVersion)
	var selected = map[int]bool{}
	for {
		for index := ModuleCore; index <= ModuleExit; index++ {
			name, _ := optionNames[index]
			fmt.Printf("%d : %s\n", index, name)
		}
		fmt.Println("Input index to select module to install, multi-modules split by ',' (like 2,3): \n")
		var input string
		fmt.Scanln(&input)
		if "" == input {
			continue
		}
		//clear
		selected = map[int]bool{}
		for _, value := range strings.Split(input, ",") {
			index, err := strconv.Atoi(value)
			if err != nil {
				fmt.Printf("Invalid input: %s\n", value)
				continue
			}
			selected[index] = true
		}
		if _, exists := selected[ModuleExit]; exists {
			return
		}
		break
	}
	var session = SessionInfo{Local: true}
	session.BinaryPath = BinaryPathName
	var username string
	var err error
	if username, err = framework.InputString("Service Owner Name", "root"); err != nil{
		return
	}
	if err = setUserInfo(&session, username);err != nil{
		fmt.Printf("set user info fail: %s\n", err.Error())
		return
	}

	if err = installBasicComponents(&session); err != nil {
		fmt.Printf("install basic components fail: %s\n", err.Error())
		return
	}
	updateAllAccess(session)
	fmt.Printf("%d modules will install...\n", len(selected))
	var allRange []PortRange
	//default ranges
	{
		const (
			GroupPortBegin = 5500
			GroupPortEnd = 5599
			ModulePortBegin = 5600
			ModulePortEnd = 5800
		)
		allRange = append(allRange, PortRange{GroupPortBegin, GroupPortEnd, "udp"})
		allRange = append(allRange, PortRange{ModulePortBegin, ModulePortEnd, "udp"})
	}
	if _, exists := selected[ModuleCell];exists{
		if err = installCellDependencyPackages();err != nil{
			fmt.Printf("install cell dependency package fail: %s\n", err.Error())
			answer, err := framework.InputString("Do you want to continue? (y/N)", "no")
			answer = strings.ToLower(answer)
			if err != nil || ("y" != answer && "yes" != answer){
				fmt.Println("installing interupted by user")
				return
			}
		}
		if err = configureNetworkForCell();err != nil{
			fmt.Printf("configure default network bridge fail: %s\n", err.Error())
			return
		}
	}
	if err = checkDefaultRoute(); err != nil{
		fmt.Printf("check default route fail: %s\n", err.Error())
		return
	}
	for index := ModuleCore; index < ModuleExit; index++ {
		if _, exists := selected[index]; exists {
			//selected
			ranges, err := optionFunctions[index](&session)
			if err != nil {
				fmt.Printf("install module %s fail: %s\n", optionNames[index], err.Error())
				return
			}
			for _, ports := range ranges {
				allRange = append(allRange, ports)
			}

		}
	}
	updateAllAccess(session)
	if 0 != len(allRange) {
		if err = enabledPortRanges(session, allRange); err != nil {
			fmt.Printf("enabled port ranges fail: %s\n", err.Error())
		}
	}
	if err = enableIPForward(); err != nil{
		fmt.Printf("enable ip forward fail: %s", err.Error())
		return
	}
	fmt.Println("all modules installed\n")
}

func checkDefaultRoute() (err error){
	routes, err := netlink.RouteList(nil, netlink.FAMILY_ALL)
	if err != nil{
		return
	}
	if 0 == len(routes){
		err = errors.New("no route available")
		return
	}
	var defaultRouteAvailable = false
	for _, route := range routes{
		if route.Dst == nil{
			defaultRouteAvailable = true
		}
	}
	if !defaultRouteAvailable{
		err = errors.New("no default route available")
		return
	}
	fmt.Printf("default route ready\n")
	return nil
}

func enableIPForward() (err error){
	const (
		CheckPath = "/proc/sys/net/ipv4/ip_forward"
		ConfigFile = "/usr/lib/sysctl.d/50-default.conf"
		EnableLine = "net.ipv4.ip_forward = 1"
	)
	{
		var file *os.File
		if file, err = os.Open(CheckPath);err != nil{
			return
		}
		var scanner = bufio.NewScanner(file)
		if !scanner.Scan(){
			err = errors.New("no content available")
			return
		}
		var data = scanner.Text()
		var current int
		current, err = strconv.Atoi(data)
		if err != nil{
			return
		}
		if current == 1{
			fmt.Println("ip_forward already enabled")
			return nil
		}else{
			fmt.Println("try enable ip_forward")
		}
	}
	{
		//write config
		var file *os.File
		file, err = os.OpenFile(ConfigFile, os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil{
			return
		}
		fmt.Fprintln(file, EnableLine)
		file.Close()
		fmt.Printf("ip_forward enabled in config %s ", ConfigFile)
	}
	{
		var cmd = exec.Command("/sbin/sysctl", "-w", "net.ipv4.ip_forward=1")
		if err = cmd.Run();err != nil{
			fmt.Printf("enable ip_forward fail: %s", err.Error())
			return
		}else{
			fmt.Println("ip_forward enabled")
		}
	}
	return
}

func setUserInfo(session *SessionInfo, userName string) (err error) {
	u, err := user.Lookup(userName)
	if err != nil{
		err = fmt.Errorf("invalid user %s", userName)
		return
	}
	var group *user.Group
	//same name first
	group, err = user.LookupGroupId(u.Gid)
	if err != nil{
		err = fmt.Errorf("invalid gid %s", u.Gid)
		return
	}
	session.User = userName
	session.UserGroup = group.Name
	if session.UID, err = strconv.Atoi(u.Uid);err != nil{
		err = fmt.Errorf("invalid uid %s", u.Uid)
		return
	}
	if session.GID, err = strconv.Atoi(group.Gid);err != nil{
		err = fmt.Errorf("invalid gid %s", group.Gid)
		return
	}
	fmt.Printf("set user %s (uid: %d), group %s (gid: %d)\n",
		session.User, session.UID,
		session.UserGroup, session.GID)
	return nil
}

func updateAllAccess(session SessionInfo){
	var cmd = exec.Command("chown", "-R", fmt.Sprintf("%s:%s", session.User, session.UserGroup),
		session.ProjectPath)
	if err := cmd.Run();err != nil{
		fmt.Printf("update access fail: %s\n", err.Error())
	}else{
		fmt.Println("all access modified")
	}
}

func installBasicComponents(session *SessionInfo) (err error) {
	const (
		RootPath = "/opt"
	)

	var projectPath = filepath.Join(RootPath, ProjectName)
	if err = ensurePath(projectPath, "project", session.UID, session.GID);err != nil{
		return
	}
	session.ProjectPath = projectPath
	if err = inputDomainConfigure(session); err != nil{
		return
	}
	if err = installRootCA(session); err != nil {
		return
	}
	return
}

func inputDomainConfigure(session *SessionInfo) (err error){
	if session.Domain, err = framework.InputString("Group Domain Name", sonar.DefaultDomain); err != nil{
		return
	}
	if session.GroupAddress, err = framework.InputMultiCastAddress("Group MultiCast Address", sonar.DefaultMulticastAddress); err != nil{
		return
	}
	if session.GroupPort, err = framework.InputNetworkPort("Group MultiCast Port", sonar.DefaultMulticastPort);err !=nil{
		return
	}
	return nil
}

func installRootCA(session *SessionInfo) (err error) {
	const (
		CertPathName         = "cert"
		TrustedPath          = "/etc/pki/ca-trust/source/anchors"
		DefaultDurationYears = 99
		RSAKeyBits           = 2048
	)
	var serialNumber = big.NewInt(1699)

	var certFileName = fmt.Sprintf("%s_ca.crt.pem", ProjectName)
	var keyFileName = fmt.Sprintf("%s_ca.key.pem", ProjectName)
	if err = ensurePath(CertPathName, "cert", session.UID, session.GID); err != nil{
		return
	}
	var generatedCertFile = filepath.Join(CertPathName, certFileName)
	var generatedKeyFile = filepath.Join(CertPathName, keyFileName)
	if _, err = os.Stat(generatedCertFile); os.IsNotExist(err) {
		//generate cert file
		var certificate = x509.Certificate{
			SerialNumber: serialNumber,
			Subject: pkix.Name{
				CommonName:   fmt.Sprintf("%s Root CA", ProjectName),
				Organization: []string{ProjectName},
			},
			NotBefore:             time.Now(),
			NotAfter:              time.Now().AddDate(DefaultDurationYears, 0, 0),
			IsCA:                  true,
			ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
			KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
			BasicConstraintsValid: true,
		}
		var privateKey *rsa.PrivateKey
		privateKey, err = rsa.GenerateKey(rand.Reader, RSAKeyBits)
		if err != nil {
			return
		}
		fmt.Printf("private key with %d bits generated\n", RSAKeyBits)
		var publicKey = privateKey.PublicKey
		var certContent []byte
		certContent, err = x509.CreateCertificate(rand.Reader, &certificate, &certificate, &publicKey, privateKey)
		if err != nil {
			return
		}
		// Public key
		var certFile *os.File
		certFile, err = os.Create(generatedCertFile)
		if err != nil {
			return
		}
		if err = pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certContent}); err != nil {
			return
		}
		if err = certFile.Close(); err != nil {
			return
		}
		fmt.Printf("cert file '%s' generated\n", generatedCertFile)

		// Private key
		var keyFile *os.File
		keyFile, err = os.OpenFile(generatedKeyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, DefaultFilePerm)
		if err != nil {
			os.Remove(generatedCertFile)
			return
		}
		if err = pem.Encode(keyFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)}); err != nil {
			os.Remove(generatedCertFile)
			return
		}
		if err = keyFile.Close(); err != nil {
			os.Remove(generatedCertFile)
			return
		}
		fmt.Printf("key file '%s' generated\n", generatedKeyFile)
		if err = updateAccess(session, generatedCertFile);err != nil{
			return
		}
		if err = updateAccess(session, generatedKeyFile);err != nil{
			return
		}
	} else {
		fmt.Printf("cert '%s', key '%s' already generated\n", generatedCertFile, generatedKeyFile)
	}

	//install path
	var installedPath = filepath.Join(session.ProjectPath, CertPathName)
	if err = ensurePath(installedPath, "cert install", session.UID, session.GID); err != nil{
		return
	}
	var installedCertFile = filepath.Join(installedPath, certFileName)
	var installedKeyFile = filepath.Join(installedPath, keyFileName)
	if _, err = os.Stat(installedCertFile); os.IsNotExist(err) {
		if err = copyFile(generatedCertFile, installedCertFile); err != nil {
			return
		} else {
			fmt.Printf("'%s' copied to '%s'\n", generatedCertFile, installedCertFile)
		}
		updateAccess(session, installedCertFile)
	} else {
		fmt.Printf("cert file '%s' already installed\n", installedCertFile)
	}
	if _, err = os.Stat(installedKeyFile); os.IsNotExist(err) {
		if err = copyFile(generatedKeyFile, installedKeyFile); err != nil {
			return
		} else {
			fmt.Printf("'%s' copied to '%s'\n", generatedKeyFile, installedKeyFile)
		}
		updateAccess(session, installedKeyFile)
	} else {
		fmt.Printf("key file '%s' already installed\n", installedKeyFile)
	}
	var trustedCertFile = filepath.Join(TrustedPath, certFileName)
	if _, err = os.Stat(trustedCertFile); os.IsNotExist(err) {
		if err = copyFile(installedCertFile, trustedCertFile); err != nil {
			return
		} else {
			fmt.Printf("'%s' copied to '%s'\n", installedCertFile, trustedCertFile)
		}
		updateAccess(session, trustedCertFile)
		var cmd = exec.Command("update-ca-trust")
		if err = cmd.Run(); err != nil {
			return
		} else {
			fmt.Printf("'%s' updated\n", trustedCertFile)
		}
	} else {
		fmt.Printf("'%s' already installed\n", trustedCertFile)
	}
	session.CACertPath = installedCertFile
	session.CAKeyPath = installedKeyFile
	return
}

func enabledPortRanges(session SessionInfo, ranges []PortRange) (err error) {
	//#firewall-cmd --permanent --direct --add-rule ipv4 filter INPUT 0 -m pkttype --pkt-type multicast -j ACCEPT
	//#firewall-cmd --zone=public --add-port=5599-5800/udp --permanent
	//#firewall-cmd --zone=public --add-port=5801-6000/tcp --permanent
	//#firewall-cmd --zone=public --add-port=15900-16000/tcp --permanent
	//#firewall-cmd --reload

	//enable multicast
	var cmd = exec.Command("firewall-cmd", "--permanent","--direct","--add-rule","ipv4","filter","INPUT","0","-m","pkttype","--pkt-type","multicast","-j","ACCEPT")
	if err = cmd.Run();err != nil{
		fmt.Printf("enable multicast warning: %s", err.Error())
	}
	for _, config := range ranges{
		if config.Begin != config.End{
			cmd = exec.Command("firewall-cmd","--zone=public", "--permanent", fmt.Sprintf("--add-port=%d-%d/%s", config.Begin, config.End, config.Protocol))
		}else{
			cmd = exec.Command("firewall-cmd","--zone=public", "--permanent", fmt.Sprintf("--add-port=%d/%s", config.Begin, config.Protocol))
		}
		if err = cmd.Run();err != nil{
			fmt.Printf("add ports warning: %s", err.Error())
		}
	}
	cmd = exec.Command("firewall-cmd","--reload")
	return  cmd.Run()
}

func ensurePath(path, name string, uid, gid int) (err error) {
	if _, err = os.Stat(path);os.IsNotExist(err){
		if err = os.MkdirAll(path, DefaultPathPerm);err != nil{
			return
		}else if err = os.Chown(path, uid, gid);err != nil{
			return
		}else{
			fmt.Printf("%s path '%s' created\n", name, path)
		}
	}
	return nil
}

func updateAccess(session *SessionInfo, path string) (err error){
	return os.Chown(path, session.UID, session.GID)
}

func enableExecuteAccess(session *SessionInfo, path string) (err error){
	if err = os.Chown(path, session.UID, session.GID); err != nil{
		return err
	}
	const (
		ExecutePerm = 0740
	)
	err = os.Chmod(path, ExecutePerm)
	return
}

func copyFile(src, dst string) error {
	var err error
	var srcfd *os.File
	var dstfd *os.File
	var srcinfo os.FileInfo

	if srcfd, err = os.Open(src); err != nil {
		return err
	}
	defer srcfd.Close()

	if dstfd, err = os.Create(dst); err != nil {
		return err
	}
	defer dstfd.Close()

	if _, err = io.Copy(dstfd, srcfd); err != nil {
		return err
	}
	if srcinfo, err = os.Stat(src); err != nil {
		return err
	}
	return os.Chmod(dst, srcinfo.Mode())
}

func copyDir(src string, dst string) error {
	var err error
	var fds []os.FileInfo
	var srcinfo os.FileInfo

	if srcinfo, err = os.Stat(src); err != nil {
		return err
	}

	if err = os.MkdirAll(dst, srcinfo.Mode()); err != nil {
		return err
	}

	if fds, err = ioutil.ReadDir(src); err != nil {
		return err
	}
	for _, fd := range fds {
		srcfp := path.Join(src, fd.Name())
		dstfp := path.Join(dst, fd.Name())

		if fd.IsDir() {
			if err = copyDir(srcfp, dstfp); err != nil {
				fmt.Println(err)
			}
		} else {
			if err = copyFile(srcfp, dstfp); err != nil {
				fmt.Println(err)
			}
		}
	}
	return nil
}