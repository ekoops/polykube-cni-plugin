package utils

func CreatePeer(serviceName, servicePort string) string {
	return serviceName + ":" + servicePort
}

func Truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}