package conf

import (
	"encoding/xml"
	"errors"
	"io/ioutil"
)

type Config struct {
	XMLName        xml.Name `xml:"config"`
	TCPConfig      TCPConfig
	ConnPoolConfig ConnPoolConfig
}

type TCPConfig struct {
	XMLName       xml.Name `xml:"TCPConfig"`
	Host          string   `xml:"Host"`
	Port          string   `xml:"Port"`
	ReadWaitTime  int      `xml:"ReadWaitTime"`
	WriteWaitTime int      `xml:"WriteWaitTime"`
}

type ConnPoolConfig struct {
	XMLName     xml.Name `xml:"ConnPoolConfig"`
	InitCap     int      `xml:"InitCap"`
	MaxCap      int      `xml:"MaxCap"`
	WaitTimeout int      `xml:"WaitTimeout"`
	IdleTimeout int      `xml:"IdleTimeout"`
}

func (config *Config) FromFile(filename string) (err error) {
	configFile, err := ioutil.ReadFile(filename)
	if err != nil {
		return
	}
	err = xml.Unmarshal(configFile, config)
	return
}

func InitializeConfig() (err error) {
	if err := globalCfg.FromFile("./conf/config.xml"); err != nil {
		return errors.New("initialize config error" + err.Error())
	}
	return nil
}
