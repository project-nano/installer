package main

import (
	"path/filepath"
	"os"
	"fmt"
	"github.com/project-nano/framework"
	"encoding/json"
	"io/ioutil"
	"crypto/tls"
	"crypto/x509"
	"math/big"
	"crypto/rsa"
	"encoding/pem"
	"crypto/x509/pkix"
	"time"
	"net"
	"crypto/rand"
)

const (
	ImagePortBegin = 5801
	ImagePortEnd   = 5849
	APIPortBegin   = 5850
	APIPortEnd     = 5869
)
func CoreInstaller(session *SessionInfo) (ranges []PortRange, err error){
	const (
		ModulePathName    = "core"
		ConfigPathName    = "config"
		CertPathName      = "cert"
		ModuleExecuteName = "core"
	)
	fmt.Println("installing core module...")
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
	if err = writeCoreDomainConfig(session, configPath);err != nil{
		return
	}
	if err = writeCoreAPIConfig(session, configPath);err != nil{
		return
	}
	var certPath = filepath.Join(workingPath, CertPathName)
	if err = writeCoreImageConfig(session, configPath, certPath);err != nil{
		return
	}
	ranges = []PortRange{{ImagePortBegin, ImagePortEnd, "tcp"}, {APIPortBegin, APIPortEnd, "tcp"}}
	fmt.Println("core module installed")
	return ranges, nil
}

func writeCoreDomainConfig(session *SessionInfo, configPath string) (err error){
	const (
		DomainConfigFileName = "domain.cfg"
	)
	type DomainConfig struct {
		Domain        string `json:"domain"`
		GroupAddress  string `json:"group_address"`
		GroupPort     int    `json:"group_port"`
		ListenAddress string `json:"listen_address"`
	}
	var configFile = filepath.Join(configPath, DomainConfigFileName)
	if _, err = os.Stat(configFile); os.IsNotExist(err) {

		var config = DomainConfig{Domain:session.Domain, GroupAddress:session.GroupAddress, GroupPort:session.GroupPort}
		if config.ListenAddress, err = framework.ChooseIPV4Address("Listen Address"); err != nil{
			return
		}
		session.LocalAddress = config.ListenAddress
		session.APIAddress = config.ListenAddress
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

func writeCoreAPIConfig(session *SessionInfo, configPath string) (err error){
	const (
		APIConfigFilename    = "api.cfg"
		DefaultAPIServePort = 5850
	)
	type APIConfig struct {
		Port int `json:"port"`
	}
	var configFile = filepath.Join(configPath, APIConfigFilename)
	if _, err = os.Stat(configFile); os.IsNotExist(err) {
		var config = APIConfig{}
		if config.Port, err = framework.InputNetworkPort(fmt.Sprintf("API Serve Port (%d ~ %d)", APIPortBegin, APIPortEnd), DefaultAPIServePort);err !=nil{
			return
		}
		session.APIPort = config.Port
		//write
		var data []byte
		data, err = json.MarshalIndent(config, "", " ")
		if err != nil {
			return
		}
		if err = ioutil.WriteFile(configFile, data, DefaultFilePerm); err != nil {
			return
		}
		fmt.Printf("api configure '%s' generated\n", configFile)
	}
	return nil
}

func writeCoreImageConfig(session *SessionInfo, configPath, certPath string) (err error){
	const (
		ImageConfigFilename  = "image.cfg"
	)
	type ImageServiceConfig struct {
		CertFile string `json:"cert_file"`
		KeyFile  string `json:"key_file"`
	}

	var configFile = filepath.Join(configPath, ImageConfigFilename)
	var certFileName = fmt.Sprintf("%s_image.crt.pem", ProjectName)
	var keyFileName = fmt.Sprintf("%s_image.key.pem", ProjectName)

	var generatedCertFile = filepath.Join(certPath, certFileName)
	var generatedKeyFile = filepath.Join(certPath, keyFileName)

	if _, err = os.Stat(configFile); os.IsNotExist(err) {
		if _, err = os.Stat(generatedCertFile); os.IsNotExist(err){
			//generate new cert
			if err = ensurePath(certPath, "image server cert", session.UID, session.GID);err != nil{
				return
			}
			if err = signImageCertificate(session.CACertPath, session.CAKeyPath, session.LocalAddress, generatedCertFile, generatedKeyFile);err != nil{
				return
			}
		}

		var config = ImageServiceConfig{generatedCertFile, generatedKeyFile}
		//write
		var data []byte
		data, err = json.MarshalIndent(config, "", " ")
		if err != nil {
			return
		}
		if err = ioutil.WriteFile(configFile, data, DefaultFilePerm); err != nil {
			return
		}
		fmt.Printf("image server configure '%s' generated\n", configFile)
	}
	return
}

func signImageCertificate(caCert, caKey, localAddress, certPath, keyPath  string) (err error){
	const (
		RSAKeyBits           = 2048
		DefaultDurationYears = 99
	)
	rootPair, err := tls.LoadX509KeyPair(caCert, caKey)
	if err != nil{
		return
	}
	rootCA, err := x509.ParseCertificate(rootPair.Certificate[0])
	if err != nil{
		return err
	}
	var serialNumber = big.NewInt(1700)
	var imageCert = x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   fmt.Sprintf("%s ImageServer", ProjectName),
			Organization: []string{ProjectName},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(DefaultDurationYears, 0, 0),
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature|x509.KeyUsageKeyEncipherment|x509.KeyUsageDataEncipherment,
		IPAddresses:           []net.IP{net.ParseIP(localAddress)},
	}
	var imagePrivate *rsa.PrivateKey
	imagePrivate, err = rsa.GenerateKey(rand.Reader, RSAKeyBits)
	if err != nil {
		return
	}
	fmt.Printf("private key with %d bits generated\n", RSAKeyBits)
	var imagePublic = imagePrivate.PublicKey
	var certContent []byte
	certContent, err = x509.CreateCertificate(rand.Reader, &imageCert, rootCA, &imagePublic, rootPair.PrivateKey)
	if err != nil {
		return
	}
	// Public key
	var certFile *os.File
	certFile, err = os.Create(certPath)
	if err != nil {
		return
	}
	if err = pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certContent}); err != nil {
		return
	}
	if err = certFile.Close(); err != nil {
		return
	}
	fmt.Printf("cert file '%s' generated\n", certPath)

	// Private key
	var keyFile *os.File
	keyFile, err = os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, DefaultFilePerm)
	if err != nil {
		os.Remove(certPath)
		return
	}
	if err = pem.Encode(keyFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(imagePrivate)}); err != nil {
		os.Remove(certPath)
		return
	}
	if err = keyFile.Close(); err != nil {
		os.Remove(certPath)
		return
	}
	fmt.Printf("key file '%s' generated\n", keyPath)
	return nil
}

