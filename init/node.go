package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/containernetworking/plugins/pkg/ip"
	router "github.com/ekoops/polykube-cni-plugin/utils/router"
	log "github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net"
	"net/url"
	"strconv"
	"strings"
)


// GetNode returns a node object describing the cluster node corresponding to the provided name
func GetNode(name string) (*v1.Node, error) {
	l := log.WithField("node", name)
	node, err := clientset.CoreV1().Nodes().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		l.WithField("detail", err).Fatal("failed to retrieve cluster node info")
		return nil, fmt.Errorf("failed to retrieve the %q cluster node info: %v", name, err)
	}
	l.Info("cluster node info retrieved")
	return node, nil
}

// ParseNodePodCIDR returns the pod CIDR of the provided node
func ParseNodePodCIDR(node *v1.Node) (*net.IPNet, error) {
	l := log.WithField("node", node.Name)
	_, podCIDR, err := net.ParseCIDR(node.Spec.PodCIDR)
	if err != nil {
		l.WithField("detail", err).Fatal("failed to parse cluster node Pod CIDR")
		return nil, fmt.Errorf("failed to parse %q cluster node Pod CIDR: %v", node.Name, err)
	}
	// making sure that the pods CIDR is IPv4
	podCIDR.IP = podCIDR.IP.To4()
	if podCIDR.IP == nil {
		l.WithField(
			"detail", "unsupported IPv6 Pod CIDR",
		).Fatal("failed to parse cluster node Pod CIDR")
		return nil, fmt.Errorf("failed to parse %q cluster node Pod CIDR: unsupported IPv6 Pod CIDR", node.Name)
	}
	l.WithField("podCIDR", podCIDR).Info("parsed cluster node Pod CIDR")
	return podCIDR, nil
}

// CalcNodePodDefaultGateway returns the pods default gateway info starting from the pod CIDR using the convention
// that the IP of the default gateway is the last IP of pod CIDR other than the broadcast address (e.g.: if the
// pod CIDR is /24, then the default gateway IP will be .254
func CalcNodePodDefaultGateway(podCIDR *net.IPNet) (*GwInfo, error) {
	// calculating the broadcast address
	subIP := podCIDR.IP
	subMask := podCIDR.Mask
	subBroadcastIP := net.IP(make([]byte, 4))
	for i := range subIP {
		subBroadcastIP[i] = subIP[i] | ^subMask[i]
	}

	// using the address preceding the broadcast address as default gateway for pods
	gwIP := ip.PrevIP(subBroadcastIP)
	gwIPNet := &net.IPNet{
		IP:   gwIP,
		Mask: subMask,
	}
	gwInfo := &GwInfo{IPNet: gwIPNet}
	log.WithFields(log.Fields{
		"podCIDR": fmt.Sprintf("%+v", podCIDR),
		"gwInfo":  fmt.Sprintf("%+v", gwInfo),
	}).Info("calculated default gateway info for Pod CIDR")
	return gwInfo, nil
}

// GetNodePodDefaultGatewayMAC returns the pods default gateway MAC obtained by querying the polycube infrastructure
func GetNodePodDefaultGatewayMAC(conf *EnvConf) (net.HardwareAddr, error) {
	r, err := GetRouter(conf.routerName)
	if err != nil {
		return nil, err
	}
	l := log.WithFields(log.Fields{
		"node":       conf.nodeName,
		"podGateway": conf.routerName,
	})
	var routerMAC net.HardwareAddr
	for _, port := range r.Ports {
		if port.Name == "to_br0" {
			routerMAC, err = net.ParseMAC(port.Mac)
			if err != nil {
				l.WithField("detail", err).Fatal("failed to parse cluster node pod default gateway mac")
				return nil, fmt.Errorf("failed to parse %q cluster node pod %q default gateway mac: %v", conf.nodeName, conf.routerName, err)
			}
			l.WithField("MAC", routerMAC).Info("cluster node pod default gateway mac retrieved")
			return routerMAC, nil
		}
	}
	l.WithFields(log.Fields{
		"port":   "to_br0",
		"detail": "port not found",
	}).Fatal("failed to retrieve cluster node pod default gateway mac")
	return nil, fmt.Errorf(
		"failed to retrieve %q cluster node pod %q default gateway mac: %q port not found",
		conf.nodeName, conf.routerName, "to_br0",
	)
}

// GetNodeExtIface returns the provided node external interface info
func GetNodeExtIface(node *v1.Node) (*Iface, error) {
	l := log.WithField("node", node.Name)
	// extracting ip of the node external interface
	var extIfaceIP net.IP
	for _, addr := range node.Status.Addresses {
		if addr.Type == v1.NodeInternalIP {
			extIfaceIP = net.ParseIP(addr.Address)
			break
		}
	}
	if extIfaceIP == nil {
		l.Fatal("failed to parse cluster node external interface IP")
		return nil, fmt.Errorf("failed to parse %q cluster node external interface IP", node.Name)
	}

	// retrieving the interfaces list
	links, err := netlink.LinkList()
	if err != nil {
		l.Fatal("failed to retrieve cluster node interfaces list")
		return nil, fmt.Errorf("failed to retrieve %q cluster node interfaces list: %v", node.Name, err)
	}

	// searching for the interface whose ip list contains the external interface ip
	for _, link := range links {
		linkName := link.Attrs().Name
		linkLog := l.WithField("interface", linkName)
		addrs, err := netlink.AddrList(link, netlink.FAMILY_V4)
		if err != nil {
			linkLog.Fatal("failed to retrieve addresses list for node interface")
			return nil, fmt.Errorf(
				"failed to retrieve addresses list for %q cluster node %q interface: %v", node.Name, linkName, err,
			)
		}
		// scanning the address list for the current interface in order to determine if the list contains
		// the external interface one
		for _, addr := range addrs {
			if addr.IP.Equal(extIfaceIP) {
				extIface := &Iface{
					IPNet: addr.IPNet,
					Link:  link,
				}
				linkLog.WithField(
					"info", fmt.Sprintf("%+v", extIface),
				).Info("obtained cluster node external interface info")
				return extIface, nil
			}
		}
	}
	l.Fatal("failed to retrieve cluster node external interface info")
	return nil, fmt.Errorf("failed to retrieve %q cluster node external interface info", node.Name)
}

// CalcNodeVtepIPNet calculates the ip and the prefix length of the Vxlan Tunnel Endpoint for the provided node.
// The address is extracted from the vtepCIDR range. It is calculated using a convention based on the node name (this
// is a temporary solution)
func CalcNodeVtepIPNet(node *v1.Node, vtepCIDR *net.IPNet) (*net.IPNet, error) {
	l := log.WithField("node", node.Name)
	// extracting the worker number (this is possible since its worker node is called worker${n})
	// TODO this is a temporary solution
	n, err := strconv.Atoi(strings.TrimPrefix(node.Name, "worker"))
	if err != nil {
		l.WithField("detail", err).Fatal("failed to extract cluster node number for Vtep IP evaluation")
		return nil, fmt.Errorf("failed to extract %q cluster node number for Vtep IP evaluation: %v", node.Name, err)
	}
	nodeVtepIP := vtepCIDR.IP
	for i := 0; i < n; i++ {
		nodeVtepIP = ip.NextIP(nodeVtepIP)
	}
	nodeVtepIPNet := &net.IPNet{
		IP:   nodeVtepIP,
		Mask: vtepCIDR.Mask,
	}
	l.WithField("vtep", fmt.Sprintf("%+v", nodeVtepIPNet)).Info("cluster node Vtep IP address calculated")
	return nodeVtepIPNet, nil
}

// CreateNodeVxlanIface creates a vxlan interface on the node associating it with the node external interface
func CreateNodeVxlanIface(name string, extIface *Iface, vtepIPNet *net.IPNet) (*Iface, error) {
	l := log.WithField("interface", name)
	extIfaceIndex := extIface.Link.Attrs().Index
	// defining the vxlan interface properties
	link_ := &netlink.Vxlan{
		LinkAttrs:    netlink.LinkAttrs{Name: name},
		VxlanId:      42,            // TODO mocked
		VtepDevIndex: extIfaceIndex, // TODO this is the index of the associated link?
		Port:         4789,
	}

	// creating the vxlan interface
	if err := netlink.LinkAdd(link_); err != nil {
		l.WithField("detail", err).Fatal("failed to create the cluster node vxlan interface")
		return nil, fmt.Errorf("failed to create the cluster node %q vxlan interface: %v", name, err)
	}

	// retrieving the vxlan interface
	// TODO is it really necessary?
	link, err := netlink.LinkByName(name)
	if err != nil {
		l.WithField("detail", err).Fatal("failed to retrieve the cluster node vxlan interface")
		return nil, fmt.Errorf("failed to retrieve the cluster node %q vxlan interface: %v", name, err)
	}

	// setting up the vxlan interface
	if err := netlink.LinkSetUp(link); err != nil {
		l.WithField("detail", err).Fatal("failed to set the cluster node vxlan interface up")
		return nil, fmt.Errorf("failed to set the cluster node %q vxlan interface up: %v", name, err)
	}

	// adding IPv4 address to the interface
	addr := &netlink.Addr{
		IPNet: vtepIPNet,
		Label: "",
	}
	l = l.WithField("address", fmt.Sprintf("%+v", vtepIPNet))
	if err = netlink.AddrAdd(link, addr); err != nil {
		l.WithField("detail", err).Fatal("failed to add IPv4 address to the cluster node vxlan interface")
		return nil, fmt.Errorf("failed to add IPv4 address to the cluster node %q vxlan interface: %v", name, err)
	}
	vxlanIface := &Iface{
		IPNet: vtepIPNet,
		Link:  link,
	}
	l.Info("cluster node vxlan interface created")
	return vxlanIface, nil
}

// GetNodeDefaultGateway returns cluster node default gateway info for the node external interface
func GetNodeDefaultGateway(extIface *Iface) (*GwInfo, error) {
	extIfaceName := extIface.Link.Attrs().Name
	l := log.WithField("interface", extIfaceName)

	// retrieving the default gateway IP address
	// > retrieving the default route
	// TODO temporary solution
	routes, err := netlink.RouteGet(net.IPv4(1, 0, 0, 0))
	if err != nil {
		l.WithField(
			"detail", err,
		).Fatal("failed to retrieve the cluster node default route through cluster node external interface")
		return nil, fmt.Errorf("failed to retrieve the cluster node default route: %v", err)
	}
	if len(routes) != 1 {
		l.WithField(
			"routes", fmt.Sprintf("%+v", routes),
		).Fatal("failed to determine a cluster node single default route")
		return nil, fmt.Errorf("failed to determine a single node default route - found routes: %+v", routes)
	}
	route := routes[0]

	// > checking that the route link index is equal to the external interface index
	routeLI := route.LinkIndex
	extIfaceLI := extIface.Link.Attrs().Index
	if routeLI != extIfaceLI {
		l.WithFields(log.Fields{
			"routeLinkIndex":    routeLI,
			"extIfaceLinkIndex": extIfaceLI,
		}).Fatal("the route link index doesn't match the external interface link index")
		return nil, fmt.Errorf(
			"the route link index doesn't match the %q external interface link index - routeLinkIndex: %d, extIfaceLinkIndex: %d",
			extIfaceName,
			routeLI,
			extIfaceLI,
		)
	}

	gwIPNet := &net.IPNet{
		IP:   route.Gw,
		Mask: extIface.IPNet.Mask, // using the same prefix length of the external interface IP address
	}

	// retrieving the default gateway MAC address
	// > retrieving the neighbor list of the external interface
	neighs, err := netlink.NeighList(extIfaceLI, netlink.FAMILY_V4)
	if err != nil {
		l.WithField("detail", err).Fatal("failed to retrieve the external interface neighbor list")
		return nil, errors.New("failed to determine default gateway mac address")
	}
	// searching for a neighbor whose IP address is the default gateway one
	for _, neigh := range neighs {
		if neigh.IP.Equal(gwIPNet.IP) {
			gwInfo := &GwInfo{
				IPNet: gwIPNet,
				MAC:   neigh.HardwareAddr,
			}
			l.WithField(
				"gwInfo", fmt.Sprintf("%+v", gwInfo),
			).Info("cluster node default gateway info retrieved")
			return gwInfo, nil
		}
	}
	l.Fatal("failed to retrieve the cluster node default gateway through cluster node external interface")
	return nil, fmt.Errorf(
		"failed to retrieve the cluster node default gateway through cluster node %q external interface", extIfaceName,
	)
}

// AddNode updates the polycube cubes configuration in order to make the provided node pods reachable
// from the current node
func AddNode(vxlanIfName string, nodeIP net.IP, nodePodCIDR *net.IPNet, nodeVtepIP net.IP) error {
	l := log.WithField("name", vxlanIfName)
	// retrieving vxlan interface
	link, err := netlink.LinkByName(vxlanIfName)
	if err != nil {
		l.WithField("detail", err).Fatal("failed to retrieve the cluster node vxlan interface")
		return fmt.Errorf("failed to retrieve the cluster node %q vxlan interface: %v", vxlanIfName, err)
	}

	// appending to bridge fdb a rule for the new node
	neigh := &netlink.Neigh{
		LinkIndex:    link.Attrs().Index, // vxlan index
		State:        netlink.NUD_PERMANENT,
		IP:           nodeIP,
		HardwareAddr: net.HardwareAddr{0, 0, 0, 0, 0, 0},
	}
	l = l.WithFields(log.Fields{
		"entry":  fmt.Sprintf("%+v", *neigh),
		"nodeIP": nodeIP,
	})
	if err := netlink.NeighAppend(neigh); err != nil {
		l.WithField(
			"detail", err,
		).Fatal("failed to configure the node fdb for allowing communication with the new node IP through the vxlan interface")
		return fmt.Errorf(
			"failed to configure the node fdb for allowing communication with the new node %q IP through the %q vxlan interface: %v",
			nodeIP, vxlanIfName, err,
		)
	}
	l.Info("node fdb configured in order to allow communication with the new node through vxlan interface")

	// adding route to router in order to make node pod CIDR reachable throw vxlan interface
	route := router.Route{
		Network:    nodePodCIDR.String(),
		Nexthop:    nodeVtepIP.String(),
		Interface_: "to_vxlan0",
	}
	l = log.WithFields(log.Fields{
		"router": "r0",
		"route":  fmt.Sprintf("%+v", route),
		"nodeIP": nodeIP,
	})

	if resp, err := routerAPI.CreateRouterRouteByID(context.TODO(), "r0", url.QueryEscape(route.Network), route.Nexthop, route); err != nil {
		l.WithFields(log.Fields{
			"error":    err,
			"response": fmt.Sprintf("%+v", resp),
		}).Fatal("failed to set router route for allowing communication with the new node IP through the vxlan interface")
		return fmt.Errorf(
			"failed to set %q router route for allowing communication with the new node %q IP through the %q vxlan"+
				"interface - error: %v, response: %+v",
			"r0", nodeIP, vxlanIfName, err, resp,
		)
	}
	l.Info("router route configured in order to allow communication with the new node through 6vxlan interface")
	return nil
}

// BuildNodeInfo returns an object describing the cluster node on which it is executed. The provided name must match
// the cluster node name on which the program is executed
func BuildNodeInfo(conf *EnvConf) (*NodeInfo, error) {
	node, err := GetNode(conf.nodeName)
	if err != nil {
		return nil, err
	}
	podCIDR, err := ParseNodePodCIDR(node)
	if err != nil {
		return nil, err
	}
	podGwInfo, err := CalcNodePodDefaultGateway(podCIDR)
	if err != nil {
		return nil, err
	}

	extIface, err := GetNodeExtIface(node)
	if err != nil {
		return nil, err
	}

	nodeVtepIPNet, err := CalcNodeVtepIPNet(node, conf.vtepCIDR)
	if err != nil {
		return nil, err
	}

	nodeGwInfo, err := GetNodeDefaultGateway(extIface)
	if err != nil {
		return nil, err
	}

	return &NodeInfo{
		name:          conf.nodeName,
		kNode:         node,
		podCIDR:       podCIDR,
		podGwInfo:     podGwInfo,
		extIface:      extIface,
		nodeVtepIPNet: nodeVtepIPNet,
		nodeGwInfo:    nodeGwInfo,
	}, nil
}
