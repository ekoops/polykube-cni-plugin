#!/bin/bash

ROOT_DIR="../"
CONF_DIR="$ROOT_DIR/conf"
BIN_DIR="$ROOT_DIR/bin"

set -x
sudo ip netns del ns1
sudo ip link del dev gw
sudo rm -r /var/lib/cni/networks/testnet
polycubectl lbrp-containeri del
polycubectl br0 del

set -e
sudo ip netns add ns1
sudo ip link add gw type dummy
sudo ip link set dev gw address aa:bb:cc:dd:ee:ff
sudo ip link set dev gw up
sudo ip addr add 10.0.1.254/24 dev gw
polycubectl simplebridge add br0

RESULT=$(sudo CNI_COMMAND=ADD \
CNI_CONTAINERID=containerid \
CNI_NETNS=/run/netns/ns1 \
CNI_IFNAME=veth1 \
CNI_PATH=$BIN_DIR \
$BIN_DIR/polykube-cni-plugin < $CONF_DIR/config.json)
set +x
if [[ ! -z "$RESULT" ]]; then
	jq --argjson RESULT "$RESULT" '. += {prevResult: $RESULT}' $CONF_DIR/config.json > $CONF_DIR/check_test_config.json
fi
