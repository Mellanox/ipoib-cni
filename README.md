# Infiniband IPoIB plugin

## Overview

Allow user to create IPoIB link and move it to the pod.

## Example configuration

```
{
	"name": "mynet",
	"type": "infiniband",
	"master": "ib0",
	"ipam": {
		"type": "dhcp"
	}
}
```

## Network configuration reference

* `name` (string, required): the name of the network
* `type` (string, required): "infiniband"
* `master` (string, required): name of the host interface to create the link from
* `mtu` (integer, optional): explicitly set MTU to the specified value. Defaults to the value chosen by the kernel.
* `ipam` (dictionary, required): IPAM configuration to be used for this network. For interface only without ip address, create empty dictionary.

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