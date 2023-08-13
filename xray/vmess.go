package xray

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/fatih/color"
	"github.com/xtls/xray-core/infra/conf"
	"net/url"
	"strings"
	"xray-knife/utils"
)

func method1(v *Vmess, link string) error {
	b64encoded := link[8:]
	decoded, err := utils.Base64Decode(b64encoded)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(decoded, v); err != nil {
		return err
	}
	return nil
}

func method2(v *Vmess, link string) error {
	fullUrl, err := url.Parse(link)
	if err != nil {
		return err
	}
	b64encodedPart := fullUrl.Host
	decoded, err := utils.Base64Decode(b64encodedPart)
	if err != nil {
		return err
	}
	suhp := strings.Split(string(decoded), ":") // security, uuid, address, port
	if len(suhp) != 3 {
		return errors.New(fmt.Sprintf("vmess unreconized: security:addr:port -> %s", suhp))
	}
	v.Security = suhp[0]
	comb := strings.Split(suhp[1], "@") // ID@ADDR
	v.ID = comb[0]
	v.Address = comb[1]
	//parseUint, err := strconv.ParseUint(suhp[2], 10, 16)
	//if err != nil {
	//	return err
	//}
	v.Port = suhp[2]

	v.Aid = "0"

	queryValues := fullUrl.Query()
	if value := queryValues.Get("remarks"); value != "" {
		v.Remark = value
	}

	if value := queryValues.Get("path"); value != "" {
		v.Path = value
	}

	if value := queryValues.Get("tls"); value == "1" {
		v.TLS = "tls"
	}

	if value := queryValues.Get("obfs"); value != "" {
		switch value {
		case "websocket":
			v.Network = "ws"
			v.Type = "none"
		case "none":
			v.Network = "tcp"
			v.Type = "none"
		}
	}
	if value := queryValues.Get("obfsParam"); value != "" {
		v.Host = value
	}
	if value := queryValues.Get("peer"); value != "" {
		v.SNI = value
	} else {
		if v.TLS == "tls" {
			v.SNI = v.Host
		}
	}

	return nil
}

func (v *Vmess) Parse(configLink string) error {
	if !strings.HasPrefix(configLink, "vmess://") {
		return fmt.Errorf("vmess unreconized: %s", configLink)
	}

	if err := method1(v, configLink); err == nil {

	} else if err = method2(v, configLink); err == nil {

	}

	v.OrigLink = configLink

	return nil
}

func (v *Vmess) DetailsStr() string {
	copyV := *v
	info := fmt.Sprintf("%s: Vmess\n%s: %s\n%s: %s\n%s: %s\n%s: %v\n%s: %s\n",
		color.RedString("Protocol"),
		color.RedString("Remark"), copyV.Remark,
		color.RedString("Network"), copyV.Network,
		color.RedString("IP"), copyV.Address,
		color.RedString("Port"), copyV.Port,
		color.RedString("UUID"), copyV.ID)

	if copyV.Network == "" {

	} else if copyV.Network == "http" || copyV.Network == "ws" || copyV.Network == "h2" {
		if copyV.Type == "" {
			copyV.Type = "none"
		}
		if copyV.Host == "" {
			copyV.Host = "none"
		}
		if copyV.Path == "" {
			copyV.Path = "none"
		}

		info += fmt.Sprintf("%s: %s\n%s: %s\n%s: %s\n",
			color.RedString("Type"), copyV.Type,
			color.RedString("Host"), copyV.Host,
			color.RedString("Path"), copyV.Path)
	} else if copyV.Network == "kcp" {
		info += fmt.Sprintf("%s: %s\n", color.RedString("KCP Seed"), copyV.Path)
	} else if copyV.Network == "grpc" {
		if copyV.Host == "" {
			copyV.Host = "none"
		}
		info += fmt.Sprintf("%s: %s\n", color.RedString("ServiceName"), copyV.Path)
	}
	if len(copyV.TLS) != 0 {
		if len(copyV.SNI) == 0 {
			copyV.SNI = copyV.Host
		}
		if len(copyV.ALPN) == 0 {
			copyV.ALPN = "none"
		}
		if len(copyV.TlsFingerprint) == 0 {
			copyV.TlsFingerprint = "none"
		}
		info += fmt.Sprintf("%s: tls\n%s: %s\n%s: %s\n%s: %s\n",
			color.RedString("TLS"),
			color.RedString("SNI"), copyV.SNI,
			color.RedString("ALPN"), copyV.ALPN,
			color.RedString("Fingerprint"), copyV.TlsFingerprint)
	}
	return info
}

func (v *Vmess) ConvertToGeneralConfig() (GeneralConfig, error) {
	var g GeneralConfig
	g.Protocol = "vmess"
	g.Address = v.Address
	g.Aid = v.Aid
	g.Host = v.Host
	g.ID = v.ID
	g.Network = v.Network
	g.Path = v.Path
	g.Port = v.Port
	g.Remark = v.Remark
	g.TLS = v.TLS
	g.SNI = v.SNI
	g.ALPN = v.ALPN
	g.TlsFingerprint = v.TlsFingerprint
	g.Type = v.Type
	g.OrigLink = v.OrigLink

	return g, nil
}

func (v *Vmess) BuildOutboundDetourConfig(allowInsecure bool) (*conf.OutboundDetourConfig, error) {
	out := &conf.OutboundDetourConfig{}
	out.Tag = "proxy"
	out.Protocol = "vmess"

	p := conf.TransportProtocol(v.Network)
	s := &conf.StreamConfig{
		Network: &p,
	}

	switch v.Network {
	case "tcp":
		s.TCPSettings = &conf.TCPConfig{}
		if v.Type == "" || v.Type == "none" {
			s.TCPSettings.HeaderConfig = json.RawMessage([]byte(`{ "type": "none" }`))
		} else {
			pathb, _ := json.Marshal(strings.Split(v.Path, ","))
			hostb, _ := json.Marshal(strings.Split(v.Host, ","))
			s.TCPSettings.HeaderConfig = json.RawMessage([]byte(fmt.Sprintf(`
			{
				"type": "http",
				"request": {
					"path": %s,
					"headers": {
						"Host": %s
					}
				}
			}
			`, string(pathb), string(hostb))))
		}
	case "kcp":
		s.KCPSettings = &conf.KCPConfig{}
		s.KCPSettings.HeaderConfig = json.RawMessage([]byte(fmt.Sprintf(`{ "type": "%s" }`, v.Type)))
	case "ws":
		s.WSSettings = &conf.WebSocketConfig{}
		s.WSSettings.Path = v.Path
		s.WSSettings.Headers = map[string]string{
			"Host": v.Host,
		}
	case "h2", "http":
		s.HTTPSettings = &conf.HTTPConfig{
			Path: v.Path,
		}
		if v.Host != "" {
			h := conf.StringList(strings.Split(v.Host, ","))
			s.HTTPSettings.Host = &h
		}
	case "grpc":
		multiMode := false
		if v.Type != "gun" {
			multiMode = true
		}
		s.GRPCConfig = &conf.GRPCConfig{
			InitialWindowsSize: 65536,
			HealthCheckTimeout: 20,
			MultiMode:          multiMode,
			IdleTimeout:        60,
			ServiceName:        v.Path,
		}
	}

	if v.TLS == "tls" {
		if v.TlsFingerprint == "" {
			v.TlsFingerprint = "chrome"
		}
		s.TLSSettings = &conf.TLSConfig{
			Fingerprint: v.TlsFingerprint,
			Insecure:    allowInsecure,
		}
		if v.SNI != "" {
			s.TLSSettings.ServerName = v.SNI
		} else {
			s.TLSSettings.ServerName = v.Host
		}
		if v.ALPN != "" {
			s.TLSSettings.ALPN = &conf.StringList{v.ALPN}
		}
	}

	out.StreamSetting = s
	oset := json.RawMessage([]byte(fmt.Sprintf(`{
  "vnext": [
    {
      "address": "%s",
      "port": %v,
      "users": [
        {
          "id": "%s",
          "alterId": %v,
          "security": "%s"
        }
      ]
    }
  ]
}`, v.Address, v.Port, v.ID, v.Aid, v.Security)))
	out.Settings = &oset
	return out, nil
}
