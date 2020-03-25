# IP Over Infiniband (IPoIB) CNI plugin

## Overview

Allow user to create IPoIB child link and move it to the pod.

## Example configuration

```
{
	"name": "mynet",
	"type": "ipoib",
	"master": "ib0",
	"ipam": {
        "type": "host-local",
        "subnet": "192.168.2.0/24",
        "rangeStart": "192.168.2.10",
        "rangeEnd": "192.168.2.216",
        "routes": [{"dst": "0.0.0.0/0"}],
        "gateway": "192.168.2.1"
    }
}
```

## Network configuration reference

* `name` (string, required): the name of the network
* `type` (string, required): "ipoib"
* `master` (string, required): name of the host interface to create the link from
* `ipam` (dictionary, required): IPAM configuration to be used for this network. For interface only without ip address, create empty dictionary, `dhcp` type is not supported.

## Multi Architecture Support
The supported architectures:
* AMD64
* PPC64LE

Buiding image for AMD64:
```
$ DOCKERFILE=Dockerfile make image 
```
Buiding image for PPC64LE:
```
$ DOCKERFILE=Dockerfile.ppc64le make image        
```
