package main

import (
	"github.com/vishvananda/netlink"
	v1 "k8s.io/api/core/v1"
	"net"
)

type EnvConf struct {
	nodeName        string
	vxlanIfName		string
	vtepCIDR        *net.IPNet
	CNIConfFilePath string
	vClusterCIDR    *net.IPNet
	MTU             int
	bridgeName      string
	routerName      string
	lbrpName        string
	k8sDispName     string
}

type NodeInfo struct {
	name          string
	kNode         *v1.Node
	podCIDR       *net.IPNet
	podGwInfo     *GwInfo
	extIface      *Iface
	nodeVtepIPNet *net.IPNet
	nodeGwInfo    *GwInfo
}

type GwInfo struct {
	IPNet *net.IPNet
	MAC   net.HardwareAddr
}

type Iface struct {
	IPNet *net.IPNet
	Link  netlink.Link
}
