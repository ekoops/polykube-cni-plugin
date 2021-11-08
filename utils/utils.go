package utils

func CreatePeer(serviceName, servicePort string) string {
	return serviceName + ":" + servicePort
}
