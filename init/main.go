package main

import (
	"context"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"net"
	"os"
	"runtime"
	"strings"
)

const (
	//kubeconfig = "/var/lib/pcn_k8s/kubeconfig.conf"
	kubeconfig = "/home/ubuntu/.kube/config"
)

var (
	clientset        *kubernetes.Clientset
)

func init() {
	log.SetLevel(log.DebugLevel)
	// TODO change log file position
	file, err := os.OpenFile("polykube.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		log.SetOutput(file)
	} else {
		// TODO what I have to do in this case? Discard?
		fmt.Println("NO LOG")
		log.SetOutput(ioutil.Discard)
	}

	// this ensures that main runs only on main thread (thread group leader).
	// since namespace ops (unshare, setns) are done for a single thread, we
	// must ensure that the goroutine does not jump from OS thread to thread
	runtime.LockOSThread()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.WithField("detail", err).Fatal("failed to build config")
		panic(fmt.Sprintf("failed to build config: %v", err))
	}

	//// creates the in-cluster config
	//config, err := rest.InClusterConfig()
	//checkError("create k8s client", err)

	// creates the clientset
	clientset, err = kubernetes.NewForConfig(config)
	if err != nil {
		log.WithField("detail", err).Fatal("failed to create clientset")
		panic(fmt.Sprintf("failed to create clientset: %v", err))

	}
}

func addOtherNodes(conf *EnvConf) error {
	nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.WithFields(log.Fields{
			"detail": err,
		}).Fatal("failed to retrieve cluster nodes info")
		return fmt.Errorf("failed to retrieve cluster nodes info: %v", err)
	}
	for _, node := range nodes.Items {
		if strings.HasPrefix(node.Name, "worker") && node.Name != conf.nodeName {
			var nodeIP net.IP
			for _, addr := range node.Status.Addresses {
				if addr.Type == v1.NodeInternalIP {
					nodeIP = net.ParseIP(addr.Address)
					break
				}
			}
			_, nodePodCIDR, err := net.ParseCIDR(node.Spec.PodCIDR)
			if err != nil {
				log.WithFields(log.Fields{
					"detail": err,
				}).Fatal("failed to parse cluster node podCIDR")
				return fmt.Errorf("failed to retrieve %q cluster node podCIDR: %v", node.Name, err)
			}
			nodeVtepIPNet, err := CalcNodeVtepIPNet(&node, conf.vtepCIDR)
			if err != nil {
				return fmt.Errorf("failed to add %q cluster node podCIDR: %v", node.Name, err)
			}
			if err := AddNode(conf.vxlanIfName, nodeIP, nodePodCIDR, nodeVtepIPNet.IP); err != nil {
				return fmt.Errorf("failed to add %q cluster node podCIDR: %v", node.Name, err)
			}
		}
	}
	return nil
}

func main() {
	conf, err := GetEnvConf()
	if err != nil {
		panic(err)
	}
	nodeInfo, err := BuildNodeInfo(conf)
	if err != nil {
		panic(err)
	}

	_, err = CreateNodeVxlanIface(conf.vxlanIfName, nodeInfo.extIface, nodeInfo.nodeVtepIPNet)
	if err != nil {
		panic(err)
	}

	if err := CreateCubes(nodeInfo, conf); err != nil {
		panic(err)
	}

	podGwMAC, err := GetNodePodDefaultGatewayMAC(conf)
	if err != nil {
		panic(err)
	}
	nodeInfo.podGwInfo.MAC = podGwMAC

	if err := CreateCNIConfFile(conf, nodeInfo); err != nil {
		panic(err)
	}
	if err := addOtherNodes(conf); err != nil {
		panic(err)
	}
}