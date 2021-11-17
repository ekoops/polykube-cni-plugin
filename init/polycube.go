package main

import (
	"context"
	"fmt"
	"github.com/ekoops/polykube-cni-plugin/utils"
	k8sdispatcher "github.com/ekoops/polykube-cni-plugin/utils/k8sdispatcher"
	lbrp "github.com/ekoops/polykube-cni-plugin/utils/lbrp"
	router "github.com/ekoops/polykube-cni-plugin/utils/router"
	simplebridge "github.com/ekoops/polykube-cni-plugin/utils/simplebridge"
	log "github.com/sirupsen/logrus"
	"net"
)

const (
	basePath = "http://127.0.0.1:9000/polycube/v1"
)

var (
	simplebridgeAPI  *simplebridge.SimplebridgeApiService
	lbrpAPI          *lbrp.LbrpApiService
	routerAPI        *router.RouterApiService
	k8sdispatcherAPI *k8sdispatcher.K8sdispatcherApiService
)

func init() {
	// init simplebrige API
	cfgSimplebridge := simplebridge.Configuration{BasePath: basePath}
	srSimplebridge := simplebridge.NewAPIClient(&cfgSimplebridge)
	simplebridgeAPI = srSimplebridge.SimplebridgeApi

	// init router API
	cfgRouter := router.Configuration{BasePath: basePath}
	srRouter := router.NewAPIClient(&cfgRouter)
	routerAPI = srRouter.RouterApi

	// init lbrp API
	cfgLbrp := lbrp.Configuration{BasePath: basePath}
	srLbrp := lbrp.NewAPIClient(&cfgLbrp)
	lbrpAPI = srLbrp.LbrpApi

	// init k8sdispatcher API
	cfgK8sdispatcher := k8sdispatcher.Configuration{BasePath: basePath}
	srK8sdispatcher := k8sdispatcher.NewAPIClient(&cfgK8sdispatcher)
	k8sdispatcherAPI = srK8sdispatcher.K8sdispatcherApi
}

// CreateBridge creates a polycube simplebridge cube
func CreateBridge(name string) error {
	l := log.WithField("name", name)
	// defining bridge port that will be connected to the router
	brToRPort := simplebridge.Ports{
		Name: "to_r0",
	}
	brPorts := []simplebridge.Ports{brToRPort}
	br := simplebridge.Simplebridge{
		Name:     name,
		Loglevel: "TRACE",
		Ports:    brPorts,
	}

	l = l.WithField("bridge", fmt.Sprintf("%+v", br))
	// creating bridge
	if resp, err := simplebridgeAPI.CreateSimplebridgeByID(context.TODO(), name, br); err != nil {
		l.WithFields(log.Fields{
			"error":    err,
			"response": fmt.Sprintf("%+v", resp),
		}).Fatal("failed to create bridge")
		return fmt.Errorf("failed to create %q bridge - error: %s, response: %+v", name, err, resp)
	}
	l.Info("bridge created")
	return nil
}

// GetRouter retrieve a polycube router cube given the name
func GetRouter(name string) (*router.Router, error) {
	l := log.WithField("name", name)

	// retrieving router
	r, resp, err := routerAPI.ReadRouterByID(context.TODO(), name)
	if err != nil {
		l.WithFields(log.Fields{
			"error":    err,
			"response": fmt.Sprintf("%+v", resp),
		}).Fatal("failed to retrieve router")
		return nil, fmt.Errorf("failed to retrieve %q router - error: %s, response: %+v", name, err, resp)
	}
	l.Info("router retrieved")
	return &r, nil
}

// CreateRouter creates a polycube router cube
func CreateRouter(name string, extIface *Iface, podsGwInfo *GwInfo, nodeGwInfo *GwInfo) error {
	l := log.WithField("name", name)

	// defining the router port that will be connected to the bridge
	rToBrPort := router.Ports{
		Name: "to_br0",
		Ip:   podsGwInfo.IPNet.String(),
		Mac:  podsGwInfo.MAC.String(),
	}
	// defining the router port that will be connected to the vxlan interface
	rToVxlanPort := router.Ports{
		Name: "to_vxlan0",
	}
	// defining the router port that will be connected to the lbrp
	rToLbrpPort := router.Ports{
		Name: "to_lbrp0",
		Ip:   extIface.IPNet.String(),
		Mac:  extIface.Link.Attrs().HardwareAddr.String(),
	}
	rPorts := []router.Ports{rToBrPort, rToVxlanPort, rToLbrpPort}

	// defining router default route and setting static arp table entry for the default gateway
	routes := []router.Route{
		{
			Network:    "0.0.0.0/0",
			Nexthop:    nodeGwInfo.IPNet.IP.String(),
			Interface_: "to_lbrp0",
		},
	}
	arptable := []router.ArpTable{
		{
			Address:    nodeGwInfo.IPNet.IP.String(),
			Mac:        nodeGwInfo.MAC.String(),
			Interface_: "to_lbrp0",
		},
	}
	r := router.Router{
		Name:     name,
		Ports:    rPorts,
		Loglevel: "TRACE",
		Route:    routes,
		ArpTable: arptable,
	}

	l = l.WithField("router", fmt.Sprintf("%+v", r))
	// creating router
	if resp, err := routerAPI.CreateRouterByID(context.TODO(), name, r); err != nil {
		l.WithFields(log.Fields{
			"error":    err,
			"response": fmt.Sprintf("%+v", resp),
		}).Fatal("failed to create router")
		return fmt.Errorf("failed to create %q router - error: %s, response: %+v", name, err, resp)
	}
	l.Info("router created")
	return nil
}

// CreateLbrp creates a polycube lbrp cube for managing incoming connection
func CreateLbrp(name string) error {
	l := log.WithField("name", name)

	// defining the lbrp port that will be connected to the router interface
	lbToRPort := lbrp.Ports{
		Name:  "to_r0",
		Type_: "backend",
	}
	// defining the lbrp port that will be connected to the k8sdispatcher interface
	lbToKPort := lbrp.Ports{
		Name:  "to_k0",
		Type_: "frontend",
	}

	lbPorts := []lbrp.Ports{lbToRPort, lbToKPort}
	lb := lbrp.Lbrp{
		Name:     name,
		Loglevel: "TRACE",
		Ports:    lbPorts,
	}

	l = l.WithField("lbrp", fmt.Sprintf("%+v", lb))
	// creating lbrp
	if resp, err := lbrpAPI.CreateLbrpByID(context.TODO(), name, lb); err != nil {
		l.WithFields(log.Fields{
			"error":    err,
			"response": fmt.Sprintf("%+v", resp),
		}).Fatal("failed to create lbrp")
		return fmt.Errorf("failed to create %q lbrp - error: %s, response: %+v", name, err, resp)
	}
	l.Info("lbrp created")
	return nil
}

// CreateK8sDispatcher creates a polycube k8sdispatcher cube for managing incoming connection
func CreateK8sDispatcher(name string, podCIDR *net.IPNet) error {
	l := log.WithField("name", name)

	// defining the k8sdispatcher port that will be connected to the lbrp interface
	kToLbPort := k8sdispatcher.Ports{
		Name:  "to_lbrp0",
		Type_: "BACKEND",
	}
	// defining the k8sdispatcher port that will be connected to the node external interface
	kToIntPort := k8sdispatcher.Ports{
		Name:  "to_int",
		Type_: "FRONTEND",
	}
	kPorts := []k8sdispatcher.Ports{
		kToLbPort,
		//kToIntPort,
	}
	k := k8sdispatcher.K8sdispatcher{
		Name:            name,
		Loglevel:        "TRACE",
		Ports:           kPorts,
		ClusterIpSubnet: "11.11.11.0/24", // TODO mocked
		ClientSubnet:    podCIDR.String(),
		InternalSrcIp:   "3.3.1.3",     // TODO mocked
		NodeportRange:   "30000-32767", // TODO mocked
	}

	l = l.WithField("k8sdispatcher", fmt.Sprintf("%+v", k))
	// creating k8sdispatcher
	if resp, err := k8sdispatcherAPI.CreateK8sdispatcherByID(context.TODO(), name, k); err != nil {
		l.WithFields(log.Fields{
			"error":    err,
			"response": fmt.Sprintf("%+v", resp),
		}).Fatal("failed to create k8sdispatcher")
		return fmt.Errorf("failed to create %q k8sdispatcher - error: %s, response: %+v", name, err, resp)
	}
	// TODO trying to create in a single shot also the following port
	if resp, err := k8sdispatcherAPI.CreateK8sdispatcherPortsByID(context.TODO(), name, "to_int", kToIntPort); err != nil {
		l.WithFields(log.Fields{
			"port":     "to_int",
			"error":    err,
			"response": fmt.Sprintf("%+v", resp),
		}).Fatal("failed to create k8sdispatcher port")
		return fmt.Errorf("failed to create %q k8sdispatcher port - error: %s, response: %+v", name, err, resp)
	}
	l.Info("k8sdispatcher created")
	return nil
}

// ConnectCubes connect each port of the already deployed polycube infrastructure with the right peer
func ConnectCubes(conf *EnvConf, extIface *Iface) error {
	brName := conf.bridgeName
	rName := conf.routerName
	lbName := conf.lbrpName
	kName := conf.k8sDispName
	// updating bridge ports
	// updating bridge "to_r0" port in order to set peer=r0:to_br0
	brToRPortName := "to_r0"
	brToRPortPeer := utils.CreatePeer(rName, "to_br0")
	l := log.WithFields(log.Fields{
		"name": brName,
		"port": brToRPortName,
		"peer": brToRPortPeer,
	})
	brToRPort := simplebridge.Ports{
		Peer: brToRPortPeer,
	}
	if resp, err := simplebridgeAPI.UpdateSimplebridgePortsByID(context.TODO(), brName, brToRPortName, brToRPort); err != nil {
		l.WithFields(log.Fields{
			"error":    err,
			"response": fmt.Sprintf("%+v", resp),
		}).Fatal("failed to set bridge port peer")
		return fmt.Errorf("failed to set %q port peer on %q bridge to %q - error: %s, response: %+v",
			brToRPortName, brName, brToRPortPeer, err, resp,
		)
	}
	l.Info("bridge port peer set")

	// updating router ports
	// updating router "to_br0" port in order to set peer=br0:to_r0
	rToBrPortName := "to_br0"
	rToBrPortPeer := utils.CreatePeer(brName, "to_r0")
	l = l.WithFields(log.Fields{
		"name": rName,
		"port": rToBrPortName,
		"peer": rToBrPortPeer,
	})
	rToBrPort := router.Ports{
		Peer: rToBrPortPeer,
	}
	if resp, err := routerAPI.UpdateRouterPortsByID(context.TODO(), rName, rToBrPortName, rToBrPort); err != nil {
		l.WithFields(log.Fields{
			"error":    err,
			"response": fmt.Sprintf("%+v", resp),
		}).Fatal("failed to set router port peer")
		return fmt.Errorf("failed to set %q port peer on %q router to %q - error: %s, response: %+v",
			rToBrPortName, rName, rToBrPortPeer, err, resp,
		)
	}
	l.Info("router port peer set")

	// updating router "to_vxlan0" port in order to set peer=vxlan0
	rToVxlanPortName := "to_vxlan0"
	rToVxlanPortPeer := "vxlan0"
	l = l.WithFields(log.Fields{
		"port": rToVxlanPortName,
		"peer": rToVxlanPortPeer,
	})
	rToVxlanPort := router.Ports{
		Peer: rToVxlanPortPeer,
	}
	if resp, err := routerAPI.UpdateRouterPortsByID(context.TODO(), rName, rToVxlanPortName, rToVxlanPort); err != nil {
		l.WithFields(log.Fields{
			"error":    err,
			"response": fmt.Sprintf("%+v", resp),
		}).Fatal("failed to set router port peer")
		return fmt.Errorf("failed to set %q port peer on %q router to %q - error: %s, response: %+v",
			rToVxlanPortName, rName, rToVxlanPortPeer, err, resp,
		)
	}
	l.Info("router port peer set")

	// updating router "to_lbrp0" port in order to set peer=lbrp0:to_r0
	rToLbPortName := "to_lbrp0"
	rToLbPortPeer := utils.CreatePeer(lbName, "to_r0")
	l = l.WithFields(log.Fields{
		"port": rToLbPortName,
		"peer": rToLbPortPeer,
	})
	rToLbPort := router.Ports{
		Peer: rToLbPortPeer,
	}
	if resp, err := routerAPI.UpdateRouterPortsByID(context.TODO(), rName, rToLbPortName, rToLbPort); err != nil {
		l.WithFields(log.Fields{
			"error":    err,
			"response": fmt.Sprintf("%+v", resp),
		}).Fatal("failed to set router port peer")
		return fmt.Errorf("failed to set %q port peer on %q router to %q - error: %s, response: %+v",
			rToLbPortName, rName, rToLbPortPeer, err, resp,
		)
	}
	l.Info("router port peer set")

	// updating lbrp ports
	// updating lbrp "to_r0" port in order to set peer=r0:to_lbrp0
	lbToRPortName := "to_r0"
	lbToRPortPeer := utils.CreatePeer(rName, "to_lbrp0")
	l = l.WithFields(log.Fields{
		"name": lbName,
		"port": rToBrPortName,
		"peer": rToBrPortPeer,
	})
	lbToRPort := lbrp.Ports{
		Peer: lbToRPortPeer,
	}
	if resp, err := lbrpAPI.UpdateLbrpPortsByID(context.TODO(), lbName, lbToRPortName, lbToRPort); err != nil {
		l.WithFields(log.Fields{
			"error":    err,
			"response": fmt.Sprintf("%+v", resp),
		}).Fatal("failed to set lbrp port peer")
		return fmt.Errorf("failed to set %q port peer on %q lbrp to %q - error: %s, response: %+v",
			lbToRPortName, lbName, lbToRPortPeer, err, resp,
		)
	}
	l.Info("lbrp port peer set")

	// updating lbrp "to_k0" port in order to set peer=k0:to_lbrp0
	lbToKPortName := "to_k0"
	lbToKPortPeer := utils.CreatePeer(kName, "to_lbrp0")
	l = l.WithFields(log.Fields{
		"port": lbToKPortName,
		"peer": rToBrPortPeer,
	})
	lbToKPort := lbrp.Ports{
		Peer: lbToKPortPeer,
	}
	if resp, err := lbrpAPI.UpdateLbrpPortsByID(context.TODO(), lbName, lbToKPortName, lbToKPort); err != nil {
		l.WithFields(log.Fields{
			"error":    err,
			"response": fmt.Sprintf("%+v", resp),
		}).Fatal("failed to set lbrp port peer")
		return fmt.Errorf("failed to set %q port peer on %q lbrp to %q - error: %s, response: %+v",
			lbToKPortName, lbName, lbToKPortPeer, err, resp,
		)
	}
	l.Info("lbrp port peer set")

	// updating k8sdispatcher ports
	// updating k8sdispatcher "to_lbrp0" port in order to set peer=lbrp0:to_k0
	kToLbPortName := "to_lbrp0"
	kToLbPortPeer := utils.CreatePeer(lbName, "to_k0")
	l = l.WithFields(log.Fields{
		"name": kName,
		"port": kToLbPortName,
		"peer": kToLbPortPeer,
	})
	kToLbPort := k8sdispatcher.Ports{
		Peer: kToLbPortPeer,
	}
	if resp, err := k8sdispatcherAPI.UpdateK8sdispatcherPortsByID(context.TODO(), kName, kToLbPortName, kToLbPort); err != nil {
		l.WithFields(log.Fields{
			"error":    err,
			"response": fmt.Sprintf("%+v", resp),
		}).Fatal("failed to set k8sdispatcher port peer")
		return fmt.Errorf("failed to set %q port peer on %q k8sdispatcher to %q - error: %s, response: %+v",
			kToLbPortName, kName, kToLbPortPeer, err, resp,
		)
	}
	l.Info("k8sdispatcher port peer set")

	// updating k8sdispatcher "to_int" port in order to set peer=external_interface_name
	kToIntPortName := "to_int"
	kToIntPortPeer := extIface.Link.Attrs().Name
	l = l.WithFields(log.Fields{
		"port": kToIntPortName,
		"peer": kToIntPortPeer,
	})
	kToIntPort := k8sdispatcher.Ports{
		Peer: kToIntPortPeer,
	}
	if resp, err := k8sdispatcherAPI.UpdateK8sdispatcherPortsByID(context.TODO(), kName, kToIntPortName, kToIntPort); err != nil {
		l.WithFields(log.Fields{
			"error":    err,
			"response": fmt.Sprintf("%+v", resp),
		}).Fatal("failed to set k8sdispatcher port peer")
		return fmt.Errorf("failed to set %q port peer on %q k8sdispatcher to %q - error: %s, response: %+v",
			kToIntPortName, kName, kToIntPortPeer, err, resp,
		)
	}
	l.Info("k8sdispatcher port peer set")

	return nil
}

func CreateCubes(nodeInfo *NodeInfo, conf *EnvConf) error {
	if err := CreateBridge(conf.bridgeName); err != nil {
		return err
	}
	if err := CreateRouter(conf.routerName, nodeInfo.extIface, nodeInfo.podGwInfo, nodeInfo.nodeGwInfo); err != nil {
		return err
	}
	if err := CreateLbrp(conf.lbrpName); err != nil {
		return err
	}
	if err := CreateK8sDispatcher(conf.k8sDispName, nodeInfo.podCIDR); err != nil {
		return err
	}
	if err := ConnectCubes(conf, nodeInfo.extIface); err != nil {
		return err
	}
	return nil
}
