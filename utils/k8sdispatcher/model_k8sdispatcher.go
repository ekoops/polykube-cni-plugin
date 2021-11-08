/*
 * k8sdispatcher API
 *
 * k8sdispatcher API generated from k8sdispatcher.yang
 *
 * API version: 1.0.0
 * Generated by: Swagger Codegen (https://github.com/swagger-api/swagger-codegen.git)
 */

package swagger

type K8sdispatcher struct {
	// Name of the k8sdispatcher service
	Name string `json:"name,omitempty"`
	// UUID of the Cube
	Uuid string `json:"uuid,omitempty"`
	// Type of the Cube (TC, XDP_SKB, XDP_DRV)
	Type_       string `json:"type,omitempty"`
	ServiceName string `json:"service-name,omitempty"`
	// Defines the logging level of a service instance, from none (OFF) to the most verbose (TRACE)
	Loglevel string `json:"loglevel,omitempty"`
	// Entry of the ports table
	Ports []Ports `json:"ports,omitempty"`
	// Defines if the service is visible in Linux
	Shadow bool `json:"shadow,omitempty"`
	// Defines if all traffic is sent to Linux
	Span bool `json:"span,omitempty"`
	// Range of VIPs where clusterIP services are exposed
	ClusterIpSubnet string `json:"cluster-ip-subnet,omitempty"`
	// Range of IPs of pods in this node
	ClientSubnet string `json:"client-subnet,omitempty"`
	// Internal src ip used for services with externaltrafficpolicy=cluster
	InternalSrcIp string         `json:"internal-src-ip,omitempty"`
	NattingRule   []NattingRule  `json:"natting-rule,omitempty"`
	NodeportRule  []NodeportRule `json:"nodeport-rule,omitempty"`
	// Port range used for NodePort services
	NodeportRange string `json:"nodeport-range,omitempty"`
}