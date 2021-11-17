package main

import (
	"fmt"
	"github.com/containernetworking/plugins/pkg/ip"
	log "github.com/sirupsen/logrus"
	"net"
	"os"
	"strconv"
)

const (
	confFormat = `
{
	"cniVersion": "0.4.0",
	"name": "mynet",
	"type": "polykube-cni-plugin",
	"MTU": %d,
	"vclustercidr": "%s",
	"bridge": "%s",
	"gateway": {
		"ip": "%s",
		"mac": "%s"
	},
	"ipam": {
		"type": "host-local",
		"ranges": [
			[
				{
					"subnet": "%s",
					"rangeStart": "%s",
					"rangeEnd": "%s",
					"gateway": "%s"
				}
			]
		],
		"dataDir": "/var/lib/cni/networks/mynet",
		"resolvConf": "/etc/resolv.conf"
	}
}
`
)

func getEnv(envVar string, defaultVal string) string {
	env := os.Getenv(envVar)
	if env == "" {
		env = defaultVal
		log.WithFields(log.Fields{
			"envVar":  envVar,
			"default": defaultVal,
		}).Warning("env variable not found. Default value applied")
	}
	return env
}

// GetEnvConf create a conf object taking values from environment variables and in some cases, if the environment
// variable is not defined, defaulting them
func GetEnvConf() (*EnvConf, error) {
	// nodeName
	conf := &EnvConf{}
	conf.nodeName = os.Getenv("NODE_K8S_NAME")
	if conf.nodeName == "" {
		log.Fatal("K8S_NODE_NAME env variable not found")
		panic("K8S_NODE_NAME env variable not found")
	}

	// vxlanIfName
	conf.vxlanIfName =  getEnv("NODE_VXLAN_IFACE_NAME", "vxlan0")

	// vtepCIDR
	_, vtepCIDR, err := net.ParseCIDR(getEnv("NODE_VTEP_CIDR", "10.18.0.0/16"))
	if err != nil {
		log.WithField(
			"detail", "NODE_VTEP_CIDR must be in the format w.x.y.z/n",
		).Fatal("failed to parse env variable")
		return nil, fmt.Errorf("failed to parse env variable: NODE_VTEP_CIDR must be in the format w.x.y.z/n")
	}
	conf.vtepCIDR = vtepCIDR

	// CNIConfFilePath
	conf.CNIConfFilePath = getEnv("CNI_CONF_FILE_PATH", "/etc/cni/net.d/00-polykube.json")

	// vClusterCIDR
	_, vClusterCIDR, err := net.ParseCIDR(getEnv("POLYCUBE_VPODS_RANGE", "10.10.0.0/16"))
	if err != nil {
		log.WithField(
			"detail", "POLYCUBE_VPODS_RANGE must be in the format w.x.y.z/n",
		).Fatal("failed to parse env variable")
		return nil, fmt.Errorf("failed to parse env variable: POLYCUBE_VPODS_RANGE must be in the format w.x.y.z/n")
	}
	conf.vClusterCIDR = vClusterCIDR

	// MTU
	MTU, err := strconv.Atoi(getEnv("POLYCUBE_MTU", "1450"))
	if err != nil {
		log.WithField("detail", "POLYCUBE_MTU must be a positive integer").Fatal("failed to parse env variable")
		return nil, fmt.Errorf("failed to parse env variable: POLYCUBE_MTU must be a positive integer")
	}
	conf.MTU = MTU


	// bridgeName
	conf.bridgeName = getEnv("POLYCUBE_BRIDGE_NAME", "br0")

	// routerName
	conf.routerName = getEnv("POLYCUBE_ROUTER_NAME", "r0")

	// lbrpName
	conf.lbrpName = getEnv("POLYCUBE_LBRP_NAME", "lbrp0")

	// k8sDispName
	conf.k8sDispName = getEnv("POLYCUBE_K8SDISP_NAME", "k0")
	return conf, nil
}

// CreateCNIConfFile creates the configuration file for the CNI plugin
func CreateCNIConfFile(conf *EnvConf, nodeInfo *NodeInfo) error {
	fName := conf.CNIConfFilePath
	f, err := os.Create(fName)
	if err != nil {
		log.WithFields(log.Fields{
			"path":   fName,
			"detail": err,
		}).Fatal("failed to create cni config file")
		return fmt.Errorf("failed to create cni config file in %q: %v", fName, err)
	}
	defer f.Close()

	podCIDR := nodeInfo.podCIDR
	podGwIP := nodeInfo.podGwInfo.IPNet.IP
	podGwMAC := nodeInfo.podGwInfo.MAC

	if _, err := fmt.Fprintf(f,
		confFormat,
		conf.MTU,
		conf.vClusterCIDR,
		conf.bridgeName,
		podGwIP.String(),
		podGwMAC.String(),
		podCIDR.String(),
		ip.NextIP(podCIDR.IP).String(), // .1
		ip.PrevIP(podGwIP).String(),    // .253
		podGwIP.String(),
	); err != nil {
		log.WithFields(log.Fields{
			"path":   fName,
			"detail": err,
		}).Fatal("failed to write cni config file")
		return fmt.Errorf("failed to write cni config file in %q: %v", fName, err)
	}
	return nil
}
