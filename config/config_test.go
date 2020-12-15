package config_test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lulugyf/sshserv/api"
	"github.com/lulugyf/sshserv/config"
	"github.com/lulugyf/sshserv/dataprovider"
	"github.com/lulugyf/sshserv/serv"
)

const (
	tempConfigName = "temp"
)

func TestLoadConfigTest(t *testing.T) {
	configDir := ".."
	err := config.LoadConfig(configDir, "")
	if err != nil {
		t.Errorf("error loading config")
	}
	emptyHTTPDConf := api.HTTPDConf{}
	if config.GetHTTPDConfig() == emptyHTTPDConf {
		t.Errorf("error loading httpd conf")
	}
	emptyProviderConf := dataprovider.Config{}
	if config.GetProviderConf() == emptyProviderConf {
		t.Errorf("error loading provider conf")
	}
	emptySFTPDConf := serv.Configuration{}
	if config.GetSFTPDConfig().BindPort == emptySFTPDConf.BindPort {
		t.Errorf("error loading SFTPD conf")
	}
	confName := tempConfigName + ".json"
	configFilePath := filepath.Join(configDir, confName)
	err = config.LoadConfig(configDir, tempConfigName)
	if err == nil {
		t.Errorf("loading a non existent config file must fail")
	}
	ioutil.WriteFile(configFilePath, []byte("{invalid json}"), 0666)
	err = config.LoadConfig(configDir, tempConfigName)
	if err == nil {
		t.Errorf("loading an invalid config file must fail")
	}
	ioutil.WriteFile(configFilePath, []byte("{\"serv\": {\"bind_port\": \"a\"}}"), 0666)
	err = config.LoadConfig(configDir, tempConfigName)
	if err == nil {
		t.Errorf("loading a config with an invalid bond_port must fail")
	}
	os.Remove(configFilePath)
}

func TestEmptyBanner(t *testing.T) {
	configDir := ".."
	confName := tempConfigName + ".json"
	configFilePath := filepath.Join(configDir, confName)
	config.LoadConfig(configDir, "")
	sftpdConf := config.GetSFTPDConfig()
	sftpdConf.Banner = " "
	c := make(map[string]serv.Configuration)
	c["serv"] = sftpdConf
	jsonConf, _ := json.Marshal(c)
	err := ioutil.WriteFile(configFilePath, jsonConf, 0666)
	if err != nil {
		t.Errorf("error saving temporary configuration")
	}
	config.LoadConfig(configDir, tempConfigName)
	sftpdConf = config.GetSFTPDConfig()
	if strings.TrimSpace(sftpdConf.Banner) == "" {
		t.Errorf("SFTPD banner cannot be empty")
	}
	os.Remove(configFilePath)
}

func TestInvalidUploadMode(t *testing.T) {
	configDir := ".."
	confName := tempConfigName + ".json"
	configFilePath := filepath.Join(configDir, confName)
	config.LoadConfig(configDir, "")
	sftpdConf := config.GetSFTPDConfig()
	sftpdConf.UploadMode = 10
	c := make(map[string]serv.Configuration)
	c["serv"] = sftpdConf
	jsonConf, _ := json.Marshal(c)
	err := ioutil.WriteFile(configFilePath, jsonConf, 0666)
	if err != nil {
		t.Errorf("error saving temporary configuration")
	}
	err = config.LoadConfig(configDir, tempConfigName)
	if err == nil {
		t.Errorf("Loading configuration with invalid upload_mode must fail")
	}
	os.Remove(configFilePath)
}
